// Package postgres configures the shared PostgreSQL connection pool.
package postgres

import (
	"context"
	"crypto/x509"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New does not require a reachable database at startup. Readiness establishes that separately.
func New(ctx context.Context, databaseURL string, queryTimeout time.Duration) (*pgxpool.Pool, error) {
	return NewWithRootCA(ctx, databaseURL, queryTimeout, "")
}

// NewWithRootCA accepts an optional PEM CA bundle for private database certificates.
// The bundle changes trust roots only; full certificate and hostname verification remains required.
func NewWithRootCA(ctx context.Context, databaseURL string, queryTimeout time.Duration, rootCAPEM string) (*pgxpool.Pool, error) {
	cfg, err := poolConfig(databaseURL, queryTimeout, rootCAPEM)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not initialize PostgreSQL pool")
	}
	return pool, nil
}

func poolConfig(databaseURL string, queryTimeout time.Duration, rootCAPEM string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL pool configuration")
	}
	if rootCAPEM != "" {
		parsed, err := url.Parse(databaseURL)
		if err != nil || parsed.Query().Get("sslmode") != "verify-full" || cfg.ConnConfig.TLSConfig == nil || cfg.ConnConfig.TLSConfig.InsecureSkipVerify || len(cfg.ConnConfig.Fallbacks) != 0 {
			return nil, errors.New("DATABASE_CA_CERT requires sslmode=verify-full without connection fallbacks")
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(rootCAPEM)) {
			return nil, errors.New("DATABASE_CA_CERT must contain PEM certificates")
		}
		cfg.ConnConfig.TLSConfig.RootCAs = roots
	}
	cfg.MaxConns = 20
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(queryTimeout.Milliseconds(), 10)
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "10000"
	return cfg, nil
}
