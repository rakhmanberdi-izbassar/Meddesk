package health

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

type pingerStub struct {
	err   error
	calls int
	ctx   context.Context
}

func (p *pingerStub) Ping(ctx context.Context) error { p.calls++; p.ctx = ctx; return p.err }

type migrationStub struct {
	state db.SchemaMigration
	err   error
	calls int
	ctx   context.Context
}

func (m *migrationStub) GetMigrationState(ctx context.Context) (db.SchemaMigration, error) {
	m.calls++
	m.ctx = ctx
	return m.state, m.err
}

func TestReadinessRequiresConnectivityAndExpectedCleanSchema(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		pingError, queryError error
		version               int64
		dirty, wantError      bool
		queryCalls            int
	}{
		{"ready", nil, nil, 4, false, false, 1},
		{"database unavailable", errors.New("secret database DSN"), nil, 4, false, true, 0},
		{"schema missing", nil, errors.New("secret database DSN"), 0, false, true, 1},
		{"schema behind", nil, nil, 3, false, true, 1},
		{"schema ahead", nil, nil, 5, false, true, 1},
		{"schema dirty", nil, nil, 4, true, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := &pingerStub{err: tc.pingError}
			queries := &migrationStub{err: tc.queryError, state: db.SchemaMigration{Version: tc.version, Dirty: tc.dirty}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := NewService(pool, queries).Ready(ctx)
			if (err != nil) != tc.wantError {
				t.Fatalf("Ready returned %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("readiness leaks database details")
			}
			if pool.calls != 1 || queries.calls != tc.queryCalls {
				t.Fatal("incorrect readiness query order")
			}
			if pool.ctx != ctx || (tc.queryCalls > 0 && queries.ctx != ctx) {
				t.Fatal("request context was not propagated")
			}
		})
	}
}
