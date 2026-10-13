package settings

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"time"
)

type Database interface {
	db.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct{ database Database }

func NewRepository(d Database) *Repository { return &Repository{d} }
func uid(s string) pgtype.UUID             { var id pgtype.UUID; _ = id.Scan(s); return id }
func (r *Repository) Get(ctx context.Context, u auth.User) (Settings, error) {
	var tx pgx.Tx
	var err error
	if d, ok := r.database.(interface {
		BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	}); ok {
		tx, err = d.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	} else {
		tx, err = r.database.Begin(ctx)
	}
	if err != nil {
		return Settings{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	a, err := q.ReportingActor(ctx, uid(u.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrForbidden
	}
	if err != nil {
		return Settings{}, err
	}
	if !a.IsActive || (a.Role != auth.RoleAdmin && a.Role != auth.RoleEngineer && a.Role != auth.RoleEmployee) {
		return Settings{}, ErrForbidden
	}
	row, err := q.GetApplicationSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{row.ClinicName, row.DefaultTicketPriority, row.ReportingTimezone, row.Version, row.UpdatedAt.Time}
	return out, tx.Commit(ctx)
}
func (r *Repository) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(&mutationRepository{db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type mutationRepository struct{ q *db.Queries }

func (r *mutationRepository) Lock(ctx context.Context) error { return r.q.LockUserManagement(ctx) }
func (r *mutationRepository) Actor(ctx context.Context, id string) (auth.User, error) {
	u, err := r.q.GetManagedUserForUpdate(ctx, uid(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, ErrForbidden
	}
	return auth.User{ID: id, Role: u.Role, IsActive: u.IsActive}, err
}
func (r *mutationRepository) Get(ctx context.Context) (Settings, error) {
	row, err := r.q.GetApplicationSettingsForUpdate(ctx)
	return Settings{row.ClinicName, row.DefaultTicketPriority, row.ReportingTimezone, row.Version, row.UpdatedAt.Time}, err
}
func (r *mutationRepository) Update(ctx context.Context, v Settings) (Settings, error) {
	row, err := r.q.UpdateApplicationSettings(ctx, db.UpdateApplicationSettingsParams{ClinicName: v.ClinicName, DefaultTicketPriority: v.DefaultTicketPriority, ReportingTimezone: v.ReportingTimezone, Version: v.Version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrVersionConflict
	}
	return Settings{row.ClinicName, row.DefaultTicketPriority, row.ReportingTimezone, row.Version, row.UpdatedAt.Time}, err
}
func (r *mutationRepository) Audit(ctx context.Context, actor string, version int32, changed []string) error {
	data, err := json.Marshal(map[string]any{"version": version, "changed_fields": changed})
	if err != nil {
		return err
	}
	return r.q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{ActorID: uid(actor), Action: "settings.updated", EntityType: "application_settings", Metadata: data})
}
