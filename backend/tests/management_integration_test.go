package tests

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/departments"
	"github.com/meddesk/meddesk/backend/internal/users"
	"golang.org/x/crypto/bcrypt"
)

func managementAdmin(t *testing.T, ctx context.Context, tx pgx.Tx) (auth.User, fixture) {
	t.Helper()
	f := createFixture(t, ctx, tx)
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='ADMIN') WHERE id=$1`, f.user)
	return auth.User{ID: f.user, Email: f.email, DisplayName: "Synthetic administrator", Role: auth.RoleAdmin, DepartmentID: f.department, IsActive: true}, f
}
func boolValue(v bool) *bool { return &v }
func TestManagedUserPersistenceFilteringAndRevocation(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := users.NewService(users.NewRepository(tx))
	password := rand.Text()
	email := rand.Text() + "@example.invalid"
	input := users.CreateInput{Email: " " + strings.ToUpper(email) + " ", DisplayName: " Synthetic employee ", Password: password, Role: auth.RoleEmployee, DepartmentID: f.department}
	employee, err := service.Create(ctx, actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if employee.Email != strings.ToLower(email) || employee.DisplayName != "Synthetic employee" || !employee.IsActive {
		t.Fatalf("incorrect user normalization: %+v", employee)
	}
	var storedHash string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, employee.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil {
		t.Fatalf("password not properly hashed: %v", err)
	}
	cost, _ := bcrypt.Cost([]byte(storedHash))
	if cost != 12 {
		t.Fatalf("expected bcrypt cost12, got %d", cost)
	}
	if _, err := service.Create(ctx, actor, input); !errors.Is(err, users.ErrConflict) {
		t.Fatalf("duplicate email: %v", err)
	}
	page, err := service.List(ctx, actor, users.Filter{Search: employee.Email, Role: auth.RoleEmployee, Active: boolValue(true), Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != employee.ID {
		t.Fatalf("incorrect list filters: %+v", page)
	}
	hash := make([]byte, 32)
	if _, err := rand.Read(hash); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `INSERT INTO sessions(token_hash,user_id,csrf_token_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, hash, employee.ID, make([]byte, 32))
	update := users.UpdateInput{Email: employee.Email, DisplayName: employee.DisplayName, Role: auth.RoleEngineer, DepartmentID: f.department, IsActive: boolValue(true)}
	employee, err = service.Update(ctx, actor, employee.ID, update)
	if err != nil {
		t.Fatal(err)
	}
	if employee.Role != auth.RoleEngineer {
		t.Fatalf("role did not change: %+v", employee)
	}
	var sessions, audits int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM sessions WHERE user_id=$1),(SELECT count(*) FROM audit_logs WHERE entity_id=$1 AND action='user.updated')`, employee.ID).Scan(&sessions, &audits); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || audits != 1 {
		t.Fatalf("revocation/audit failed: sessions=%d audits=%d", sessions, audits)
	}
	profile, err := service.Profile(ctx, employee, users.ProfileInput{DisplayName: "Жаңа атау"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != employee.ID || profile.Role != auth.RoleEngineer || profile.DisplayName != "Жаңа атау" {
		t.Fatalf("incorrect profile update: %+v", profile)
	}
	if _, err := service.List(ctx, employee, users.Filter{Limit: 20}); !errors.Is(err, users.ErrForbidden) {
		t.Fatalf("engineer received admin user list: %v", err)
	}
}
func TestDepartmentManagementActiveUserPolicy(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, _ := managementAdmin(t, ctx, tx)
	deptService := departments.NewService(departments.NewRepository(tx))
	userService := users.NewService(users.NewRepository(tx))
	name := "Synthetic department " + rand.Text()
	d, err := deptService.Create(ctx, actor, departments.CreateInput{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deptService.Create(ctx, actor, departments.CreateInput{Name: strings.ToUpper(name)}); !errors.Is(err, departments.ErrConflict) {
		t.Fatalf("case-insensitive department uniqueness: %v", err)
	}
	employee, err := userService.Create(ctx, actor, users.CreateInput{Email: rand.Text() + "@example.invalid", DisplayName: "Synthetic", Password: rand.Text(), Role: auth.RoleEmployee, DepartmentID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deptService.Update(ctx, actor, d.ID, departments.UpdateInput{Name: name, IsActive: boolValue(false)}); !errors.Is(err, departments.ErrHasActiveUsers) {
		t.Fatalf("deactivated occupied department: %v", err)
	}
	if _, err := userService.Update(ctx, actor, employee.ID, users.UpdateInput{Email: employee.Email, DisplayName: employee.DisplayName, Role: employee.Role, DepartmentID: d.ID, IsActive: boolValue(false)}); err != nil {
		t.Fatal(err)
	}
	d, err = deptService.Update(ctx, actor, d.ID, departments.UpdateInput{Name: name, IsActive: boolValue(false)})
	if err != nil {
		t.Fatal(err)
	}
	if d.IsActive {
		t.Fatal("empty department remained active")
	}
	if _, err := userService.Create(ctx, actor, users.CreateInput{Email: rand.Text() + "@example.invalid", DisplayName: "Synthetic", Password: rand.Text(), Role: auth.RoleEmployee, DepartmentID: d.ID}); !errors.Is(err, users.ErrInactiveDepartment) {
		t.Fatalf("created active user in inactive department: %v", err)
	}
	if _, err := userService.Update(ctx, actor, employee.ID, users.UpdateInput{Email: employee.Email, DisplayName: employee.DisplayName, Role: employee.Role, DepartmentID: d.ID, IsActive: boolValue(true)}); !errors.Is(err, users.ErrInactiveDepartment) {
		t.Fatalf("activated user in inactive department: %v", err)
	}
	page, err := deptService.List(ctx, actor, departments.Filter{Search: name, Active: boolValue(false), Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].IsActive {
		t.Fatalf("incorrect department filters: %+v", page)
	}
}
func TestManagementMutationRollsBackWhenAuditFails(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := users.NewService(users.NewRepository(tx))
	employee, err := service.Create(ctx, actor, users.CreateInput{Email: rand.Text() + "@example.invalid", DisplayName: "Before", Password: rand.Text(), Role: auth.RoleEmployee, DepartmentID: f.department})
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `CREATE FUNCTION pg_temp.reject_management_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_management_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_management_audit()`)
	_, err = service.Profile(ctx, employee, users.ProfileInput{DisplayName: "After"})
	if err == nil {
		t.Fatal("audit failure was ignored")
	}
	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM users WHERE id=$1`, employee.ID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Before" {
		t.Fatalf("profile committed without audit: %q", name)
	}
}
