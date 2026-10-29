package school

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct{ database Database }

func NewRepository(database Database) *Repository { return &Repository{database} }
func uuid(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(value)
	if err != nil || !id.Valid {
		return id, ErrInvalid
	}
	return id, nil
}
func (r *Repository) Access(ctx context.Context, actor string) (bool, error) {
	id, err := uuid(actor)
	if err != nil {
		return false, ErrForbidden
	}
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	return db.New(tx).SchoolOwner(ctx, id)
}

// Every operation checks explicit ownership inside the same transaction. Clinic
// ADMIN never bypasses this check. All rows in this module are school-only.
func (r *Repository) authorized(ctx context.Context, actor string, fn func(*db.Queries) error) error {
	id, err := uuid(actor)
	if err != nil {
		return ErrForbidden
	}
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if _, err := q.SchoolAuthorize(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		return err
	}
	if err = fn(q); err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) {
			switch pgerr.Code {
			case "23505":
				return ErrConflict
			case "23503", "23514", "22003":
				return ErrInvalid
			}
		}
		return err
	}
	return tx.Commit(ctx)
}
