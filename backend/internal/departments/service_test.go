package departments

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"strings"
	"testing"
)

const fixtureID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const fixtureDept = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

func admin() auth.User         { return auth.User{ID: fixtureID, Role: auth.RoleAdmin, IsActive: true} }
func boolPointer(v bool) *bool { return &v }

type serviceFixture struct {
	activeUsers                             int64
	currentActive, updated, locked, audited bool
	actor                                   auth.User
}

func (f *serviceFixture) List(context.Context, Filter) (Page, error) { return Page{}, nil }
func (f *serviceFixture) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	return fn(f)
}
func (f *serviceFixture) Lock(context.Context) error                       { f.locked = true; return nil }
func (f *serviceFixture) Actor(context.Context, string) (auth.User, error) { return f.actor, nil }
func (f *serviceFixture) Get(context.Context, string) (Department, error) {
	return Department{ID: fixtureDept, Name: "Synthetic", IsActive: f.currentActive}, nil
}
func (f *serviceFixture) ActiveUsers(context.Context, string) (int64, error) {
	return f.activeUsers, nil
}
func (f *serviceFixture) Create(_ context.Context, name string) (Department, error) {
	return Department{ID: fixtureDept, Name: name, IsActive: true}, nil
}
func (f *serviceFixture) Update(_ context.Context, id, name string, active bool) (Department, error) {
	f.updated = true
	return Department{ID: id, Name: name, IsActive: active}, nil
}
func (f *serviceFixture) Audit(context.Context, string, string, string) error {
	f.audited = true
	return nil
}
func TestValidationAndAuthorizationBeforeStorage(t *testing.T) {
	service := NewService(nil)
	if _, err := service.Create(context.Background(), auth.User{Role: auth.RoleEngineer, IsActive: true}, CreateInput{Name: "IT"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("engineer created department: %v", err)
	}
	for _, name := range []string{"", strings.Repeat("ә", 151), "Bad\nname"} {
		if _, err := service.Create(context.Background(), admin(), CreateInput{Name: name}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid name %q: %v", name, err)
		}
	}
	if _, err := service.Update(context.Background(), admin(), fixtureDept, UpdateInput{Name: "IT"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing active status: %v", err)
	}
}
func TestDepartmentDeactivationRequiresNoActiveUsers(t *testing.T) {
	f := &serviceFixture{activeUsers: 1, currentActive: true, actor: admin()}
	if _, err := NewService(f).Update(context.Background(), admin(), fixtureDept, UpdateInput{Name: "IT", IsActive: boolPointer(false)}); !errors.Is(err, ErrHasActiveUsers) {
		t.Fatalf("department policy: %v", err)
	}
	if f.updated {
		t.Fatal("department was deactivated")
	}
	f.activeUsers = 0
	department, err := NewService(f).Update(context.Background(), admin(), fixtureDept, UpdateInput{Name: "IT", IsActive: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if department.IsActive || !f.locked || !f.audited {
		t.Fatalf("missing transaction steps: %+v", f)
	}
}
func TestStaleAdministratorCannotChangeDepartment(t *testing.T) {
	f := &serviceFixture{actor: auth.User{Role: auth.RoleEmployee, IsActive: true}}
	if _, err := NewService(f).Create(context.Background(), admin(), CreateInput{Name: "IT"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale role: %v", err)
	}
}
