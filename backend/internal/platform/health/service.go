package health

import (
	"context"
	"errors"

	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

const ExpectedMigrationVersion int64 = 4

type Pinger interface{ Ping(context.Context) error }
type MigrationReader interface {
	GetMigrationState(context.Context) (db.SchemaMigration, error)
}

// Service checks infrastructure state without leaking database error details to clients.
type Service struct {
	pool    Pinger
	queries MigrationReader
}

func NewService(pool Pinger, queries MigrationReader) *Service {
	return &Service{pool: pool, queries: queries}
}

func (s *Service) Ready(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return errors.New("database unavailable")
	}
	state, err := s.queries.GetMigrationState(ctx)
	if err != nil {
		return errors.New("migration state unavailable")
	}
	if state.Dirty || state.Version != ExpectedMigrationVersion {
		return errors.New("database migration state incompatible")
	}
	return nil
}
