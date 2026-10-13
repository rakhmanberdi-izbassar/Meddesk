package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool, db.New(pool)}
}

func (r *PostgresRepository) FindUserByEmail(ctx context.Context, email string) (Credential, error) {
	row, err := r.queries.AuthFindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrUnauthenticated
	}
	if err != nil {
		return Credential{}, err
	}
	return Credential{User: User{row.ID, row.Email, row.DisplayName, row.Role, row.DepartmentID, row.DepartmentName, row.IsActive}, PasswordHash: row.PasswordHash}, nil
}
func (r *PostgresRepository) FindSession(ctx context.Context, hash []byte) (Session, error) {
	row, err := r.queries.AuthFindSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrUnauthenticated
	}
	if err != nil {
		return Session{}, err
	}
	return Session{User: User{row.ID, row.Email, row.DisplayName, row.Role, row.DepartmentID, row.DepartmentName, row.IsActive}, CSRFHash: row.CsrfTokenHash, ExpiresAt: row.ExpiresAt.Time}, nil
}

// CreateSession serializes with account changes, preventing a password reset or
// deactivation between credential verification and issuing a usable session.
func (r *PostgresRepository) CreateSession(ctx context.Context, credential Credential, tokenHash, csrfHash, oldHash []byte, expires time.Time) (User, error) {
	userID, err := parseUUID(credential.User.ID)
	if err != nil {
		return User{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, err
	}
	defer rollback(ctx, tx)
	q := r.queries.WithTx(tx)
	user, err := q.AuthLockLoginUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	if !user.IsActive || user.PasswordHash != credential.PasswordHash {
		return User{}, ErrInvalidCredentials
	}
	if len(oldHash) != 0 {
		if _, err := q.AuthDeleteSession(ctx, oldHash); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return User{}, err
		}
	}
	if err := q.AuthDeleteExpiredUserSessions(ctx, userID); err != nil {
		return User{}, err
	}
	if err := q.AuthCreateSession(ctx, db.AuthCreateSessionParams{TokenHash: tokenHash, UserID: userID, CsrfTokenHash: csrfHash, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}}); err != nil {
		return User{}, err
	}
	if err := q.AuthRecordEvent(ctx, db.AuthRecordEventParams{UserID: userID, Action: "auth.login"}); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return User{user.ID, user.Email, user.DisplayName, user.Role, user.DepartmentID, user.DepartmentName, user.IsActive}, nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, hash []byte) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	q := r.queries.WithTx(tx)
	// Match account management's users-before-sessions locking order. The lookup
	// itself has no row lock; after acquiring the user lock a reset may already
	// have revoked the session, which is a harmless unauthenticated result.
	owner, err := q.AuthFindSessionOwner(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if _, err := q.AuthLockLoginUser(ctx, owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthenticated
		}
		return err
	}
	id, err := q.AuthDeleteSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	userID, err := parseUUID(id)
	if err != nil {
		return err
	}
	if err := q.AuthRecordEvent(ctx, db.AuthRecordEventParams{UserID: userID, Action: "auth.logout"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return id, fmt.Errorf("invalid stored user identifier: %w", err)
	}
	return id, nil
}

var _ Repository = (*PostgresRepository)(nil)

// Rollback remains possible after a request cancellation, with a bounded cleanup
// deadline so an unavailable database cannot retain the handler indefinitely.
func rollback(ctx context.Context, tx pgx.Tx) {
	clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(clean)
}
