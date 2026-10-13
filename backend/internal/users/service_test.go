package users

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"strings"
	"testing"
)

const adminID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const employeeID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
const departmentID = "cccccccc-cccc-cccc-cccc-cccccccccccc"

func adminActor() auth.User    { return auth.User{ID: adminID, Role: auth.RoleAdmin, IsActive: true} }
func boolPointer(v bool) *bool { return &v }
func updateInput() UpdateInput {
	return UpdateInput{Email: "employee@example.invalid", DisplayName: "Synthetic employee", Role: auth.RoleEmployee, DepartmentID: departmentID, IsActive: boolPointer(true)}
}

type serviceFixture struct {
	users                             map[string]auth.User
	departmentActive                  bool
	adminCount                        int64
	revoked, updated, audited, locked bool
	auditErr                          error
}

func (f *serviceFixture) List(context.Context, Filter) (Page, error) { return Page{}, nil }
func (f *serviceFixture) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	return fn(f)
}
func (f *serviceFixture) Lock(context.Context) error { f.locked = true; return nil }
func (f *serviceFixture) Get(_ context.Context, id string) (auth.User, error) {
	u, ok := f.users[id]
	if !ok {
		return auth.User{}, ErrNotFound
	}
	return u, nil
}
func (f *serviceFixture) ActiveDepartment(context.Context, string) (bool, error) {
	return f.departmentActive, nil
}
func (f *serviceFixture) ActiveAdministrators(context.Context) (int64, error) {
	return f.adminCount, nil
}
func (f *serviceFixture) Create(_ context.Context, m AccountMutation) (auth.User, error) {
	return auth.User{ID: employeeID, Email: m.Email, DisplayName: m.DisplayName, Role: m.Role, DepartmentID: m.DepartmentID, IsActive: true}, nil
}
func (f *serviceFixture) Update(_ context.Context, id string, m AccountMutation) (auth.User, error) {
	f.updated = true
	u := auth.User{ID: id, Email: m.Email, DisplayName: m.DisplayName, Role: m.Role, DepartmentID: m.DepartmentID, IsActive: m.IsActive}
	f.users[id] = u
	return u, nil
}
func (f *serviceFixture) Profile(_ context.Context, id, name string) (auth.User, error) {
	u := f.users[id]
	u.DisplayName = name
	f.users[id] = u
	return u, nil
}
func (f *serviceFixture) RevokeSessions(context.Context, string) error { f.revoked = true; return nil }
func (f *serviceFixture) Audit(context.Context, string, string, string, map[string]any) error {
	f.audited = true
	return f.auditErr
}
func newFixture() *serviceFixture {
	return &serviceFixture{users: map[string]auth.User{adminID: adminActor(), employeeID: {ID: employeeID, Email: "employee@example.invalid", Role: auth.RoleEmployee, DepartmentID: departmentID, IsActive: true}}, departmentActive: true, adminCount: 2}
}
func TestServiceRejectsForbiddenAndInvalidInputBeforeStorage(t *testing.T) {
	service := NewService(nil)
	employee := auth.User{ID: employeeID, Role: auth.RoleEmployee, IsActive: true}
	if _, err := service.List(context.Background(), employee, Filter{Limit: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("employee list: %v", err)
	}
	tests := []UpdateInput{updateInput(), updateInput(), updateInput(), updateInput()}
	tests[0].Role = "SUPERUSER"
	tests[1].Email = "bad"
	tests[2].DepartmentID = "' OR true"
	tests[3].IsActive = nil
	for _, input := range tests {
		if _, err := service.Update(context.Background(), adminActor(), employeeID, input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid input reached store: %v", err)
		}
	}
	input := updateInput()
	input.Role = auth.RoleEmployee
	if _, err := service.Update(context.Background(), adminActor(), strings.ToUpper(adminID), input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("uppercase self-demotion accepted: %v", err)
	}
	if _, err := service.Profile(context.Background(), employee, ProfileInput{DisplayName: strings.Repeat("ә", 151)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized profile: %v", err)
	}
}
func TestUserRoleChangeRevokesSessionsAndAudits(t *testing.T) {
	f := newFixture()
	input := updateInput()
	input.Role = auth.RoleEngineer
	user, err := NewService(f).Update(context.Background(), adminActor(), employeeID, input)
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != auth.RoleEngineer || !f.revoked || !f.audited || !f.locked {
		t.Fatalf("mutation missing security step: %+v", f)
	}
}
func TestUserDepartmentPolicyAndStaleActor(t *testing.T) {
	f := newFixture()
	f.departmentActive = false
	if _, err := NewService(f).Update(context.Background(), adminActor(), employeeID, updateInput()); !errors.Is(err, ErrInactiveDepartment) {
		t.Fatalf("inactive department: %v", err)
	}
	if f.updated {
		t.Fatal("user changed before department validation")
	}
	f = newFixture()
	current := f.users[adminID]
	current.Role = auth.RoleEmployee
	f.users[adminID] = current
	if _, err := NewService(f).Update(context.Background(), adminActor(), employeeID, updateInput()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale admin role allowed: %v", err)
	}
}
func TestLastAdministratorCannotBeRemoved(t *testing.T) {
	f := newFixture()
	target := f.users[employeeID]
	target.Role = auth.RoleAdmin
	f.users[employeeID] = target
	f.adminCount = 1
	input := updateInput()
	input.Role = auth.RoleEmployee
	if _, err := NewService(f).Update(context.Background(), adminActor(), employeeID, input); !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("last administrator: %v", err)
	}
	if f.updated {
		t.Fatal("last administrator was updated")
	}
}
func TestProfileCanOnlyChangeOwnDisplayName(t *testing.T) {
	f := newFixture()
	actor := f.users[employeeID]
	user, err := NewService(f).Profile(context.Background(), actor, ProfileInput{DisplayName: " Жаңа атау "})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != employeeID || user.DisplayName != "Жаңа атау" || user.Role != auth.RoleEmployee || f.revoked || !f.audited {
		t.Fatalf("profile changed protected account properties: %+v", user)
	}
}
func TestAuditFailureIsReturned(t *testing.T) {
	f := newFixture()
	f.auditErr = errors.New("audit unavailable")
	if _, err := NewService(f).Profile(context.Background(), f.users[employeeID], ProfileInput{DisplayName: "Updated"}); !errors.Is(err, f.auditErr) {
		t.Fatalf("audit failure lost: %v", err)
	}
}
