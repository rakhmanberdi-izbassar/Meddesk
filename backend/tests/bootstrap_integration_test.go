package tests

import (
	"errors"
	"testing"

	"github.com/meddesk/meddesk/backend/internal/bootstrap"
	"golang.org/x/crypto/bcrypt"
)

func bootstrapInput() bootstrap.Input {
	return bootstrap.Input{Email: "BOOTSTRAP@example.invalid", DisplayName: "Synthetic administrator", Department: "Synthetic bootstrap department", Password: "synthetic-bootstrap-password"}
}

func TestBootstrapCreatesFirstAdminAndRefusesRepeat(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	if err := bootstrap.Run(ctx, tx, bootstrapInput()); err != nil {
		t.Fatal(err)
	}
	var email, role, hash string
	var active bool
	if err := tx.QueryRow(ctx, `SELECT u.email,r.code,u.password_hash,u.is_active FROM users u JOIN roles r ON r.id=u.role_id`).Scan(&email, &role, &hash, &active); err != nil {
		t.Fatal(err)
	}
	if email != "bootstrap@example.invalid" || role != "ADMIN" || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(bootstrapInput().Password)) != nil {
		t.Fatal("incorrect administrator persistence")
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != 12 {
		t.Fatal("incorrect bootstrap password cost")
	}
	input := bootstrapInput()
	input.Password = "different-synthetic-password"
	if err := bootstrap.Run(ctx, tx, input); !errors.Is(err, bootstrap.ErrNotEmpty) {
		t.Fatal("repeat bootstrap was allowed")
	}
	var persisted string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE email='bootstrap@example.invalid'`).Scan(&persisted); err != nil || persisted != hash {
		t.Fatal("repeat changed existing credentials")
	}
	var users, departments, audits int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM departments),(SELECT count(*) FROM audit_logs)`).Scan(&users, &departments, &audits); err != nil {
		t.Fatal(err)
	}
	if users != 1 || departments != 1 || audits != 2 {
		t.Fatal("unexpected bootstrap records/audits")
	}
}

func TestBootstrapInactiveDepartmentRejected(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	if _, err := tx.Exec(ctx, `INSERT INTO departments(name,is_active) VALUES($1,false)`, bootstrapInput().Department); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Run(ctx, tx, bootstrapInput()); !errors.Is(err, bootstrap.ErrInactiveDepartment) {
		t.Fatal("inactive department was accepted")
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected bootstrap created a user")
	}
}

func TestBootstrapAuditFailureRollsBackDepartmentAndUser(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	execSQL(t, ctx, tx, `CREATE FUNCTION pg_temp.reject_bootstrap_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_bootstrap_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_bootstrap_audit()`)
	if err := bootstrap.Run(ctx, tx, bootstrapInput()); !errors.Is(err, bootstrap.ErrStorage) {
		t.Fatal("audit failure was ignored")
	}
	var users, departments int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM departments)`).Scan(&users, &departments); err != nil || users != 0 || departments != 0 {
		t.Fatal("bootstrap committed without its audit")
	}
}
