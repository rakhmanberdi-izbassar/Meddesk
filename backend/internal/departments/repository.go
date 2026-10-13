package departments

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
	Actor(context.Context, string) (auth.User, error)
	Get(context.Context, string) (Department, error)
	ActiveUsers(context.Context, string) (int64, error)
	Create(context.Context, string) (Department, error)
	Update(context.Context, string, string, bool) (Department, error)
	Audit(context.Context, string, string, string) error
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
	rows, err := r.queries.ListManagedDepartments(ctx, db.ListManagedDepartmentsParams{Search: f.Search, Active: active, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset)})
	if err != nil {
		return Page{}, err
	}
	total, err := r.queries.CountManagedDepartments(ctx, db.CountManagedDepartmentsParams{Search: f.Search, Active: active})
	if err != nil {
		return Page{}, err
	}
	items := make([]Department, 0, len(rows))
	for _, row := range rows {
		items = append(items, department(row))
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
func (r *mutationRepository) Actor(ctx context.Context, id string) (auth.User, error) {
	u, err := r.q.GetManagedUserForUpdate(ctx, mustUUID(id))
	if err != nil {
		return auth.User{}, mapError(err)
	}
	return auth.User{ID: uuidText(u.ID), Role: u.Role, IsActive: u.IsActive}, nil
}
func (r *mutationRepository) Get(ctx context.Context, id string) (Department, error) {
	row, err := r.q.GetManagedDepartmentForUpdate(ctx, mustUUID(id))
	return department(row), mapError(err)
}
func (r *mutationRepository) ActiveUsers(ctx context.Context, id string) (int64, error) {
	return r.q.CountActiveDepartmentUsers(ctx, mustUUID(id))
}
func (r *mutationRepository) Create(ctx context.Context, name string) (Department, error) {
	row, err := r.q.CreateManagedDepartment(ctx, name)
	return department(row), mapError(err)
}
func (r *mutationRepository) Update(ctx context.Context, id, name string, active bool) (Department, error) {
	row, err := r.q.UpdateManagedDepartment(ctx, db.UpdateManagedDepartmentParams{ID: mustUUID(id), Name: name, IsActive: active})
	return department(row), mapError(err)
}
func (r *mutationRepository) Audit(ctx context.Context, actor, action, id string) error {
	metadata, _ := json.Marshal(map[string]any{})
	return r.q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{ActorID: mustUUID(actor), Action: action, EntityType: "department", EntityID: mustUUID(id), Metadata: metadata})
}
func department(row db.Department) Department {
	return Department{ID: uuidText(row.ID), Name: row.Name, IsActive: row.IsActive, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
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
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
