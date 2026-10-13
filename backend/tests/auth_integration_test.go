package tests

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meddesk/meddesk/backend/internal/auth"
)

// Auth repositories own transactions. These committed synthetic fixtures are
// removed afterward and must run against the same disposable migrated test DB.
func authFixture(t *testing.T) (context.Context, *pgxpool.Pool, *auth.Service, *auth.PostgresRepository, string, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; auth integration requires a disposable migrated database")
	}
	// Repeated race-instrumented bcrypt comparisons need a portable scenario
	// budget on slower CI machines; this is not a production request timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var departmentID, userID, marker string
	if err := pool.QueryRow(ctx, `INSERT INTO departments(name) VALUES ('Auth fixture ' || gen_random_uuid()) RETURNING id::text`).Scan(&departmentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if userID != "" {
			if _, err := pool.Exec(clean, `DELETE FROM audit_logs WHERE actor_id=$1::uuid OR entity_id=$1::uuid`, userID); err != nil {
				t.Error(err)
			}
			if _, err := pool.Exec(clean, `DELETE FROM users WHERE id=$1::uuid`, userID); err != nil {
				t.Error(err)
			}
		}
		if _, err := pool.Exec(clean, `DELETE FROM departments WHERE id=$1::uuid`, departmentID); err != nil {
			t.Error(err)
		}
	})
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	email := marker + "@example.invalid"
	password := "synthetic-auth-fixture"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,display_name,password_hash,role_id,department_id) VALUES ($1,'Synthetic auth fixture',$2,(SELECT id FROM roles WHERE code='EMPLOYEE'),$3::uuid) RETURNING id::text`, email, hash, departmentID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	repo := auth.NewRepository(pool)
	service := auth.NewService(repo, 8*time.Hour)
	return ctx, pool, service, repo, email, password
}

func TestAuthRepositorySessionRotationFreshRoleExpiryAndLogout(t *testing.T) {
	ctx, pool, service, _, email, password := authFixture(t)
	first, err := service.Login(ctx, email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(first.Token))
	csrfHash := sha256.Sum256([]byte(first.CSRFToken))
	var storedHash, storedCSRF []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash,csrf_token_hash FROM sessions WHERE user_id=$1::uuid`, first.User.ID).Scan(&storedHash, &storedCSRF); err != nil {
		t.Fatal(err)
	}
	if string(storedHash) != string(hash[:]) || string(storedCSRF) != string(csrfHash[:]) {
		t.Fatal("session stored unexpected secrets")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='IT_ENGINEER') WHERE id=$1::uuid`, first.User.ID); err != nil {
		t.Fatal(err)
	}
	session, err := service.Authenticate(ctx, first.Token)
	if err != nil || session.User.Role != auth.RoleEngineer {
		t.Fatalf("fresh role: %+v %v", session.User, err)
	}
	second, err := service.Login(ctx, email, password, first.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, first.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("rotated session survived")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1::uuid`, first.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, second.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("inactive user authenticated")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET is_active=true WHERE id=$1::uuid`, first.User.ID); err != nil {
		t.Fatal(err)
	}
	hash = sha256.Sum256([]byte(second.Token))
	if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at=created_at+interval '1 microsecond' WHERE token_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, second.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("expired DB session authenticated")
	}
	third, err := service.Login(ctx, email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(ctx, third.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, third.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("logout did not revoke DB session")
	}
	var sessions, logins, logouts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1::uuid`, first.User.ID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE action='auth.login'),count(*) FILTER(WHERE action='auth.logout') FROM audit_logs WHERE actor_id=$1::uuid`, first.User.ID).Scan(&logins, &logouts); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || logins != 3 || logouts != 1 {
		t.Fatalf("unexpected sessions/audit counts: %d/%d/%d", sessions, logins, logouts)
	}
}

func TestAuthRepositorySessionRotationRollsBackOnInsertFailure(t *testing.T) {
	ctx, _, service, repo, email, password := authFixture(t)
	first, err := service.Login(ctx, email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(ctx, email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := repo.FindUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	oldHash := sha256.Sum256([]byte(first.Token))
	duplicateHash := sha256.Sum256([]byte(second.Token))
	csrfHash := sha256.Sum256([]byte("synthetic-csrf-hash"))
	if _, err := repo.CreateSession(ctx, credential, duplicateHash[:], csrfHash[:], oldHash[:], time.Now().Add(time.Hour)); err == nil {
		t.Fatal("duplicate token insert accepted")
	}
	if _, err := service.Authenticate(ctx, first.Token); err != nil {
		t.Fatalf("failed transaction revoked old session: %v", err)
	}
	if _, err := service.Authenticate(ctx, second.Token); err != nil {
		t.Fatal(err)
	}
}

func TestAuthRepositoryAccountLockPreventsConcurrentDeactivationLogin(t *testing.T) {
	ctx, pool, _, repo, email, _ := authFixture(t)
	credential, err := repo.FindUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1::uuid`, credential.User.ID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	hash := sha256.Sum256([]byte("synthetic-session"))
	csrfHash := sha256.Sum256([]byte("synthetic-csrf"))
	go func() {
		close(started)
		_, err := repo.CreateSession(ctx, credential, hash[:], csrfHash[:], nil, time.Now().Add(time.Hour))
		done <- err
	}()
	<-started
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("concurrent deactivation should reject session, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal("session creation did not unblock")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1::uuid`, credential.User.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("session issued during deactivation")
	}
}

func TestAuthRepositoryPasswordChangedAfterVerification(t *testing.T) {
	ctx, pool, _, repo, email, _ := authFixture(t)
	credential, err := repo.FindUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := auth.HashPassword("synthetic-password-reset")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET password_hash=$1 WHERE id=$2::uuid`, replacement, credential.User.ID); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("synthetic-session"))
	csrfHash := sha256.Sum256([]byte("synthetic-csrf"))
	if _, err := repo.CreateSession(ctx, credential, hash[:], csrfHash[:], nil, time.Now().Add(time.Hour)); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("stale password state accepted: %v", err)
	}
}

func TestAuthRepositoryLogoutUsesAccountBeforeSessionLock(t *testing.T) {
	ctx, pool, service, _, email, password := authFixture(t)
	login, err := service.Login(ctx, email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id=$1::uuid FOR UPDATE`, login.User.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- service.Logout(ctx, login.Token) }()
	// Confirm logout is waiting at the user lock, rather than holding a session
	// lock and waiting on the audit FK. This makes the deadlock regression
	// deterministic without relying on a goroutine scheduling delay.
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '-- name: AuthLockLoginUser%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("logout should wait on held account lock: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("logout did not wait at account lock; it may hold the session lock in reverse order")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deleteCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := tx.Exec(deleteCtx, `DELETE FROM sessions WHERE user_id=$1::uuid`, login.User.ID); err != nil {
		t.Fatalf("account update must acquire session lock while logout waits: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("concurrent session revocation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("logout did not finish after account transaction committed")
	}
}
