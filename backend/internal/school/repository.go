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
	return r.transaction(ctx, actor, false, func(q *db.Queries, _ string) error { return fn(q) })
}
func (r *Repository) member(ctx context.Context, actor string, fn func(*db.Queries, string) error) error {
	return r.transaction(ctx, actor, true, fn)
}
func (r *Repository) transaction(ctx context.Context, actor string, member bool, fn func(*db.Queries, string) error) error {
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
	name := ""
	if member {
		record, e := q.SchoolMemberAuthorize(ctx, id)
		err = e
		name = record.DisplayName
	} else {
		_, err = q.SchoolAuthorize(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if err = fn(q, name); err != nil {
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
