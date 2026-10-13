package tests

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

// Every fixture is rolled back. Still use a disposable, migrated database:
// identity sequences advance even when the surrounding transaction rolls back.
func databaseTransaction(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; PostgreSQL integration tests require a disposable migrated database")
	}
	// This fixture also hosts management scenarios with several bcrypt calls.
	// Keep a bounded budget that is usable with the race detector on slower CI.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return ctx, tx
}

type fixture struct {
	department string
	user       string
	category   string
	article    string
	ticket     string
	email      string
}

func createFixture(t *testing.T, ctx context.Context, tx pgx.Tx) fixture {
	t.Helper()
	var f fixture
	queryID(t, ctx, tx, &f.department, `INSERT INTO departments (name) VALUES ('Fixture ' || gen_random_uuid()) RETURNING id::text`)
	var marker string
	queryID(t, ctx, tx, &marker, `SELECT gen_random_uuid()::text`)
	f.email = marker + "@example.invalid"
	// Storage-shape fixture only; this is not a usable account/password seed.
	hash := "$2a$12$" + strings.Repeat("a", 53)
	queryID(t, ctx, tx, &f.user, `INSERT INTO users (email, display_name, password_hash, role_id, department_id)
		VALUES ($1, 'Synthetic fixture', $2, (SELECT id FROM roles WHERE code = 'EMPLOYEE'), $3)
		RETURNING id::text`, f.email, hash, f.department)
	queryID(t, ctx, tx, &f.category, `INSERT INTO knowledge_categories (slug, name_kk, name_ru)
		VALUES ($1, 'Демо', 'Демо') RETURNING id::text`, "fixture-"+marker)
	queryID(t, ctx, tx, &f.article, `INSERT INTO knowledge_articles
		(slug, title, description, category_id, symptoms, solution_steps, author_id)
		VALUES ($1, 'NCALayer бағдарламасын іске қосу', 'Принтеры не печатают', $2,
		'ЭЦҚ қосылмайды', '["Қайта іске қосу", "Перезапустите приложение"]', $3) RETURNING id::text`,
		"fixture-"+marker, f.category, f.user)
	queryID(t, ctx, tx, &f.ticket, `INSERT INTO tickets (title, description, requester_id, department_id, category_id)
		VALUES ('Synthetic issue', 'Synthetic description', $1, $2, $3) RETURNING id::text`, f.user, f.department, f.category)
	return f
}

func queryID(t *testing.T, ctx context.Context, tx pgx.Tx, target *string, sql string, args ...any) {
	t.Helper()
	if err := tx.QueryRow(ctx, sql, args...).Scan(target); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
}

func execSQL(t *testing.T, ctx context.Context, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("execute fixture SQL: %v", err)
	}
}

func TestMigratedSchemaReadiness(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	state, err := db.New(tx).GetMigrationState(ctx)
	if err != nil {
		t.Fatalf("generated migration state query: %v", err)
	}
	if state.Version != 4 || state.Dirty {
		t.Fatalf("expected clean migration 4; got %+v", state)
	}
	var roles int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM roles WHERE code IN ('EMPLOYEE', 'IT_ENGINEER', 'ADMIN')`).Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if roles != 3 {
		t.Fatalf("expected all 3 reference roles, got %d", roles)
	}
}

func TestSchemaRejectsInvalidRecords(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	f := createFixture(t, ctx, tx)
	tests := []struct {
		name string
		sql  string
		args []any
		code string
	}{
		{"case insensitive email", `INSERT INTO users (email, display_name, password_hash, role_id, department_id)
			SELECT upper(email), display_name, password_hash, role_id, department_id FROM users WHERE id = $1`, []any{f.user}, "23505"},
		{"plaintext password", `UPDATE users SET password_hash = 'plaintext' WHERE id = $1`, []any{f.user}, "23514"},
		{"unsupported bcrypt cost", `UPDATE users SET password_hash = $2 WHERE id = $1`, []any{f.user, "$2a$99$" + strings.Repeat("a", 53)}, "23514"},
		{"missing role", `UPDATE users SET role_id = 32767 WHERE id = $1`, []any{f.user}, "23503"},
		{"empty steps", `UPDATE knowledge_articles SET solution_steps = '[]' WHERE id = $1`, []any{f.article}, "23514"},
		{"wrong step type", `UPDATE knowledge_articles SET solution_steps = '[1]' WHERE id = $1`, []any{f.article}, "23514"},
		{"blank step", `UPDATE knowledge_articles SET solution_steps = '["   "]' WHERE id = $1`, []any{f.article}, "23514"},
		{"nonarray steps", `UPDATE knowledge_articles SET solution_steps = '{}' WHERE id = $1`, []any{f.article}, "23514"},
		{"publication timestamp", `UPDATE knowledge_articles SET is_published = true WHERE id = $1`, []any{f.article}, "23514"},
		{"resolution timestamp", `UPDATE tickets SET status = 'RESOLVED' WHERE id = $1`, []any{f.ticket}, "23514"},
		{"unsupported status", `UPDATE tickets SET status = 'DONE' WHERE id = $1`, []any{f.ticket}, "23514"},
		{"unsupported priority", `UPDATE tickets SET priority = 'URGENT' WHERE id = $1`, []any{f.ticket}, "23514"},
		{"missing requester", `UPDATE tickets SET requester_id = gen_random_uuid() WHERE id = $1`, []any{f.ticket}, "23503"},
		{"immutable ticket number", `UPDATE tickets SET number = DEFAULT WHERE id = $1`, []any{f.ticket}, "23514"},
		{"short session hash", `INSERT INTO sessions (token_hash, csrf_token_hash, user_id, expires_at)
			VALUES ($1, $2, $3, now() + interval '1 hour')`, []any{[]byte{1}, make([]byte, 32), f.user}, "23514"},
		{"expired session", `INSERT INTO sessions (token_hash, csrf_token_hash, user_id, expires_at)
			VALUES ($1, $2, $3, now() - interval '1 hour')`, []any{make([]byte, 32), make([]byte, 32), f.user}, "23514"},
		{"media path traversal", `INSERT INTO media_files (uploader_id, storage_key, original_filename, content_type, byte_count, sha256, purpose)
			VALUES ($1, '../outside.png', 'screen.png', 'image/png', 100, $2, 'TICKET_ATTACHMENT')`, []any{f.user, make([]byte, 32)}, "23514"},
		{"unsupported media", `INSERT INTO media_files (uploader_id, storage_key, original_filename, content_type, byte_count, sha256, purpose)
			VALUES ($1, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png', 'screen.png', 'text/html', 100, $2, 'TICKET_ATTACHMENT')`, []any{f.user, make([]byte, 32)}, "23514"},
		{"oversize image", `INSERT INTO media_files (uploader_id, storage_key, original_filename, content_type, byte_count, sha256, purpose)
			VALUES ($1, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png', 'screen.png', 'image/png', 10485761, $2, 'TICKET_ATTACHMENT')`, []any{f.user, make([]byte, 32)}, "23514"},
		{"audit metadata shape", `INSERT INTO audit_logs (action, entity_type, metadata) VALUES ('test', 'ticket', '[]')`, nil, "23514"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A failed statement aborts its PostgreSQL transaction; savepoints keep
			// the fixture usable while independently checking each constraint.
			sp, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sp.Rollback(ctx) }()
			_, err = sp.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tt.code {
				t.Fatalf("expected SQLSTATE %s; got %v", tt.code, err)
			}
		})
	}
}

func TestMultilingualSearchFoundation(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	f := createFixture(t, ctx, tx)
	var kazakh, russian, solution, trigram bool
	err := tx.QueryRow(ctx, `SELECT
		search_simple @@ plainto_tsquery('simple', 'бағдарламасын'),
		search_russian @@ plainto_tsquery('russian', 'принтер'),
		search_simple @@ plainto_tsquery('simple', 'Қайта'),
		title % 'NCALayer бағдарламасын іске қосу'
		FROM knowledge_articles WHERE id = $1`, f.article).Scan(&kazakh, &russian, &solution, &trigram)
	if err != nil {
		t.Fatalf("query generated vectors/trigram extension: %v", err)
	}
	if !kazakh || !russian || !solution || !trigram {
		t.Fatalf("missing search foundation: Kazakh=%v Russian=%v steps=%v trigram=%v", kazakh, russian, solution, trigram)
	}
	execSQL(t, ctx, tx, `UPDATE knowledge_articles SET title = 'Браузерді жаңарту' WHERE id = $1`, f.article)
	var changed bool
	if err := tx.QueryRow(ctx, `SELECT search_simple @@ plainto_tsquery('simple', 'Браузерді')
		FROM knowledge_articles WHERE id = $1`, f.article).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("generated search vector did not track title update")
	}
}

func TestTicketRelationshipsAndTimestampUpdate(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	f := createFixture(t, ctx, tx)
	var created, before, after time.Time
	if err := tx.QueryRow(ctx, `SELECT created_at, updated_at FROM tickets WHERE id = $1`, f.ticket).Scan(&created, &before); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `UPDATE tickets SET status = 'IN_PROGRESS', version = version + 1 WHERE id = $1`, f.ticket)
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM tickets WHERE id = $1`, f.ticket).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.After(before) {
		t.Fatalf("update timestamp did not advance: before=%v after=%v", before, after)
	}
	execSQL(t, ctx, tx, `UPDATE tickets SET status = 'RESOLVED', resolved_at = clock_timestamp(), version = version + 1 WHERE id = $1`, f.ticket)
	execSQL(t, ctx, tx, `INSERT INTO ticket_articles (ticket_id, article_id) VALUES ($1, $2)`, f.ticket, f.article)
	execSQL(t, ctx, tx, `INSERT INTO ticket_comments (ticket_id, author_id, body, is_internal) VALUES ($1, $2, 'Synthetic internal note', true)`, f.ticket, f.user)
	execSQL(t, ctx, tx, `INSERT INTO ticket_status_history (ticket_id, from_status, to_status, actor_id) VALUES ($1, 'IN_PROGRESS', 'RESOLVED', $2)`, f.ticket, f.user)
	var related, comments, history, version int
	var resolved time.Time
	err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ticket_articles WHERE ticket_id = t.id),
		(SELECT count(*) FROM ticket_comments WHERE ticket_id = t.id AND is_internal),
		(SELECT count(*) FROM ticket_status_history WHERE ticket_id = t.id), version, resolved_at
		FROM tickets t WHERE id = $1`, f.ticket).Scan(&related, &comments, &history, &version, &resolved)
	if err != nil {
		t.Fatal(err)
	}
	if related != 1 || comments != 1 || history != 1 || version != 3 || resolved.Before(created) {
		t.Fatalf("invalid persistence: related=%d comments=%d history=%d version=%d resolved=%v", related, comments, history, version, resolved)
	}
	execSQL(t, ctx, tx, `DELETE FROM tickets WHERE id = $1`, f.ticket)
	var children int
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ticket_articles WHERE ticket_id = $1) +
		(SELECT count(*) FROM ticket_comments WHERE ticket_id = $1) +
		(SELECT count(*) FROM ticket_status_history WHERE ticket_id = $1)`, f.ticket).Scan(&children); err != nil {
		t.Fatal(err)
	}
	if children != 0 {
		t.Fatalf("ticket child records did not cascade: %d", children)
	}
}

func TestFeedbackAttemptDeduplication(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	f := createFixture(t, ctx, tx)
	var attempt string
	queryID(t, ctx, tx, &attempt, `SELECT gen_random_uuid()::text`)
	execSQL(t, ctx, tx, `INSERT INTO article_feedback (article_id, user_id, attempt_id, helpful) VALUES ($1, $2, $3, true)`, f.article, f.user, attempt)
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `INSERT INTO article_feedback (article_id, user_id, attempt_id, helpful) VALUES ($1, $2, $3, false)`, f.article, f.user, attempt)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate attempt was not rejected: %v", err)
	}
	if err := sp.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `INSERT INTO article_feedback (article_id, user_id, attempt_id, helpful) VALUES ($1, $2, gen_random_uuid(), false)`, f.article, f.user)
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM article_feedback WHERE user_id = $1 AND article_id = $2`, f.user, f.article).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two independent attempts, got %d", count)
	}
}
