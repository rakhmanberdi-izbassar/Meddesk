package audit

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/analytics"
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
func uuid(v pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", v.Bytes[:4], v.Bytes[4:6], v.Bytes[6:8], v.Bytes[8:10], v.Bytes[10:])
}
func uuidPointer(v pgtype.UUID) *string {
	if !v.Valid {
		return nil
	}
	s := uuid(v)
	return &s
}
func authorize(ctx context.Context, q *db.Queries, u auth.User) error {
	a, err := q.ReportingActor(ctx, uid(u.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if !a.IsActive || a.Role != auth.RoleAdmin {
		return ErrForbidden
	}
	return nil
}
func (r *Repository) List(ctx context.Context, u auth.User, f Filter, now time.Time) (Page, error) {
	tx, err := analytics.BeginSnapshot(ctx, r.database)
	if err != nil {
		return Page{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err := authorize(ctx, q, u); err != nil {
		return Page{}, err
	}
	zone := f.Timezone
	if zone == "" {
		v, err := q.GetApplicationSettings(ctx)
		if err != nil {
			return Page{}, err
		}
		zone = v.ReportingTimezone
	}
	period, err := analytics.ResolvePeriod(f.From, f.To, zone, now)
	if err != nil {
		return Page{}, ErrInvalidInput
	}
	start, end := pgtype.Timestamptz{Time: period.StartAt, Valid: true}, pgtype.Timestamptz{Time: period.EndAt, Valid: true}
	rows, err := q.ListAuditLogs(ctx, db.ListAuditLogsParams{StartAt: start, EndAt: end, ActorID: uid(f.ActorID), Action: f.Action, EntityType: f.EntityType, EntityID: uid(f.EntityID), PageLimit: int32(f.Limit), PageOffset: int32(f.Offset)})
	if err != nil {
		return Page{}, err
	}
	total, err := q.CountAuditLogs(ctx, db.CountAuditLogsParams{StartAt: start, EndAt: end, ActorID: uid(f.ActorID), Action: f.Action, EntityType: f.EntityType, EntityID: uid(f.EntityID)})
	if err != nil {
		return Page{}, err
	}
	out := Page{Period: period, Items: []Entry{}, Total: total, Limit: f.Limit, Offset: f.Offset}
	for _, v := range rows {
		var name *string
		if v.ActorName.Valid {
			n := v.ActorName.String
			name = &n
		}
		out.Items = append(out.Items, Entry{v.ID, uuidPointer(v.ActorID), name, v.Action, v.EntityType, uuidPointer(v.EntityID), SanitizeMetadata(v.Action, v.Metadata), v.CreatedAt.Time})
	}
	return out, tx.Commit(ctx)
}
func (r *Repository) Filters(ctx context.Context, u auth.User) (Filters, error) {
	tx, err := analytics.BeginSnapshot(ctx, r.database)
	if err != nil {
		return Filters{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err := authorize(ctx, q, u); err != nil {
		return Filters{}, err
	}
	actions, err := q.AuditFilterActions(ctx)
	if err != nil {
		return Filters{}, err
	}
	entities, err := q.AuditFilterEntityTypes(ctx)
	if err != nil {
		return Filters{}, err
	}
	actors, err := q.AuditFilterActors(ctx)
	if err != nil {
		return Filters{}, err
	}
	out := Filters{Actions: actions, EntityTypes: entities, Actors: []Actor{}}
	for _, v := range actors {
		out.Actors = append(out.Actors, Actor{uuid(v.ID), v.DisplayName})
	}
	return out, tx.Commit(ctx)
}
