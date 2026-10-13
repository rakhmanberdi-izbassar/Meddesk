package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
)

type Database interface {
	db.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type MutationStore interface {
	Lock(context.Context) error
	Get(context.Context, string) (auth.User, error)
	ActiveDepartment(context.Context, string) (bool, error)
	ActiveAdministrators(context.Context) (int64, error)
	Create(context.Context, AccountMutation) (auth.User, error)
	Update(context.Context, string, AccountMutation) (auth.User, error)
	Profile(context.Context, string, string) (auth.User, error)
	RevokeSessions(context.Context, string) error
	Audit(context.Context, string, string, string, map[string]any) error
}
type Store interface {
	List(context.Context, Filter) (Page, error)
	Transaction(context.Context, func(MutationStore) error) error
}
type Repository struct {
	database Database
	queries  *db.Queries
}

func NewRepository(database Database) *Repository { return &Repository{database, db.New(database)} }

func (r *Repository) List(ctx context.Context, f Filter) (Page, error) {
	active := pgtype.Bool{}
	if f.Active != nil {
		active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	rows, err := r.queries.ListManagedUsers(ctx, db.ListManagedUsersParams{Search: f.Search, Role: f.Role, Active: active, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset)})
	if err != nil {
		return Page{}, err
	}
	total, err := r.queries.CountManagedUsers(ctx, db.CountManagedUsersParams{Search: f.Search, Role: f.Role, Active: active})
	if err != nil {
		return Page{}, err
	}
	items := make([]auth.User, 0, len(rows))
	for _, row := range rows {
		items = append(items, auth.User{ID: uuidText(row.ID), Email: row.Email, DisplayName: row.DisplayName, Role: row.Role, DepartmentID: uuidText(row.DepartmentID), DepartmentName: row.DepartmentName, IsActive: row.IsActive})
	}
	return Page{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}
func (r *Repository) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := fn(&mutationRepository{db.New(tx)}); err != nil {
		return mapError(err)
	}
	return mapError(tx.Commit(ctx))
}

type mutationRepository struct{ q *db.Queries }

func (r *mutationRepository) Lock(ctx context.Context) error { return r.q.LockUserManagement(ctx) }
func (r *mutationRepository) Get(ctx context.Context, id string) (auth.User, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return auth.User{}, err
	}
	u, err := r.q.GetManagedUserForUpdate(ctx, uid)
	if err != nil {
		return auth.User{}, mapError(err)
	}
	return auth.User{ID: uuidText(u.ID), Email: u.Email, DisplayName: u.DisplayName, Role: u.Role, DepartmentID: uuidText(u.DepartmentID), DepartmentName: u.DepartmentName, IsActive: u.IsActive}, nil
}
func (r *mutationRepository) ActiveDepartment(ctx context.Context, id string) (bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return false, err
	}
	d, err := r.q.GetManagedDepartmentForUpdate(ctx, uid)
	return d.IsActive, mapError(err)
}
func (r *mutationRepository) ActiveAdministrators(ctx context.Context) (int64, error) {
	return r.q.CountActiveAdministrators(ctx)
}
func (r *mutationRepository) Create(ctx context.Context, m AccountMutation) (auth.User, error) {
	id, err := r.q.CreateManagedUser(ctx, db.CreateManagedUserParams{Email: m.Email, DisplayName: m.DisplayName, PasswordHash: *m.PasswordHash, Role: m.Role, DepartmentID: mustUUID(m.DepartmentID)})
	if err != nil {
		return auth.User{}, mapError(err)
	}
	return r.Get(ctx, uuidText(id))
}
func (r *mutationRepository) Update(ctx context.Context, id string, m AccountMutation) (auth.User, error) {
	hash := pgtype.Text{}
	if m.PasswordHash != nil {
		hash = pgtype.Text{String: *m.PasswordHash, Valid: true}
	}
	err := r.q.UpdateManagedUser(ctx, db.UpdateManagedUserParams{ID: mustUUID(id), Email: m.Email, DisplayName: m.DisplayName, Role: m.Role, DepartmentID: mustUUID(m.DepartmentID), IsActive: m.IsActive, PasswordHash: hash})
	if err != nil {
		return auth.User{}, mapError(err)
	}
	return r.Get(ctx, id)
}
func (r *mutationRepository) Profile(ctx context.Context, id, name string) (auth.User, error) {
	if err := r.q.UpdateOwnProfile(ctx, db.UpdateOwnProfileParams{ID: mustUUID(id), DisplayName: name}); err != nil {
		return auth.User{}, mapError(err)
	}
	return r.Get(ctx, id)
}
func (r *mutationRepository) RevokeSessions(ctx context.Context, id string) error {
	return r.q.RevokeManagedUserSessions(ctx, mustUUID(id))
}
func (r *mutationRepository) Audit(ctx context.Context, actor, action, id string, metadata map[string]any) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return r.q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{ActorID: mustUUID(actor), Action: action, EntityType: "user", EntityID: mustUUID(id), Metadata: payload})
}
func parseUUID(id string) (pgtype.UUID, error) {
	var uid pgtype.UUID
	if !httpx.ValidUUID(id) || uid.Scan(id) != nil || !uid.Valid {
		return uid, ErrInvalidInput
	}
	return uid, nil
}
func mustUUID(id string) pgtype.UUID { uid, _ := parseUUID(id); return uid }
func uuidText(uid pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", uid.Bytes[0:4], uid.Bytes[4:6], uid.Bytes[6:8], uid.Bytes[8:10], uid.Bytes[10:16])
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrConflict
		}
		if pgErr.Code == "23503" {
			return ErrNotFound
		}
	}
	return err
}
