package settings

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"strings"
	"testing"
)

const adminID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

func admin() auth.User     { return auth.User{ID: adminID, Role: auth.RoleAdmin, IsActive: true} }
func ptr(v string) *string { return &v }

type fixture struct {
	current               Settings
	actor                 auth.User
	lock, updates, audits int
	changed               []string
	auditErr              error
}

func (f *fixture) Get(context.Context, auth.User) (Settings, error) { return f.current, nil }
func (f *fixture) Transaction(_ context.Context, fn func(MutationStore) error) error {
	before := f.current
	err := fn(&mutation{f})
	if err != nil {
		f.current = before
	}
	return err
}

type mutation struct{ f *fixture }

func (m *mutation) Lock(context.Context) error                       { m.f.lock++; return nil }
func (m *mutation) Actor(context.Context, string) (auth.User, error) { return m.f.actor, nil }
func (m *mutation) Get(context.Context) (Settings, error)            { return m.f.current, nil }
func (m *mutation) Update(_ context.Context, v Settings) (Settings, error) {
	v.Version++
	m.f.current = v
	m.f.updates++
	return v, nil
}
func (m *mutation) Audit(_ context.Context, _ string, _ int32, changed []string) error {
	m.f.audits++
	m.f.changed = changed
	return m.f.auditErr
}
func setup() (*Service, *fixture) {
	f := &fixture{current: Settings{ClinicName: "MedDesk", DefaultTicketPriority: "MEDIUM", ReportingTimezone: "Asia/Almaty", Version: 1}, actor: admin()}
	return NewService(f), f
}
func TestSettingsVersionFreshAuthorizationAndAuditRollback(t *testing.T) {
	s, f := setup()
	ctx := context.Background()
	out, err := s.Update(ctx, admin(), UpdateInput{Version: 1, ClinicName: ptr(" Clinic "), DefaultTicketPriority: ptr("HIGH")})
	if err != nil || out.ClinicName != "Clinic" || out.Version != 2 || f.lock != 1 || f.audits != 1 || len(f.changed) != 2 {
		t.Fatalf("update: %+v %v %+v", out, err, f)
	}
	_, err = s.Update(ctx, admin(), UpdateInput{Version: 1, ClinicName: ptr("Stale")})
	if !errors.Is(err, ErrVersionConflict) || f.updates != 1 {
		t.Fatal("stale version changed settings", err)
	}
	f.actor.Role = auth.RoleEngineer
	_, err = s.Update(ctx, admin(), UpdateInput{Version: 2, ClinicName: ptr("Unauthorized")})
	if !errors.Is(err, ErrForbidden) || f.updates != 1 {
		t.Fatal("stale admin authorized", err)
	}
	f.actor = admin()
	f.actor.IsActive = false
	_, err = s.Update(ctx, admin(), UpdateInput{Version: 2, ClinicName: ptr("Inactive")})
	if !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive actor authorized", err)
	}
	f.actor = admin()
	f.auditErr = errors.New("audit unavailable")
	_, err = s.Update(ctx, admin(), UpdateInput{Version: 2, ClinicName: ptr("Rollback")})
	if !errors.Is(err, f.auditErr) || f.current.ClinicName != "Clinic" || f.current.Version != 2 {
		t.Fatalf("audit failure did not roll back: %+v %v", f.current, err)
	}
}
func TestSettingsValidateInputBeforeMutation(t *testing.T) {
	for _, in := range []UpdateInput{{Version: 1}, {Version: 0, ClinicName: ptr("Clinic")}, {Version: 1, ClinicName: ptr("")}, {Version: 1, ClinicName: ptr(strings.Repeat("ә", 151))}, {Version: 1, ClinicName: ptr("a\x00b")}, {Version: 1, DefaultTicketPriority: ptr("URGENT")}, {Version: 1, ReportingTimezone: ptr("Local")}, {Version: 1, ReportingTimezone: ptr("../UTC")}, {Version: 1, ReportingTimezone: ptr("Mars/Olympus")}} {
		s, f := setup()
		if _, err := s.Update(context.Background(), admin(), in); !errors.Is(err, ErrInvalidInput) || f.lock != 0 {
			t.Fatalf("invalid input reached mutation: %+v %v", in, err)
		}
	}
	s, f := setup()
	u := admin()
	u.Role = auth.RoleEmployee
	if _, err := s.Update(context.Background(), u, UpdateInput{Version: 1, ClinicName: ptr("Clinic")}); !errors.Is(err, ErrForbidden) || f.lock != 0 {
		t.Fatal("employee mutation", err)
	}
	u.IsActive = false
	if _, err := s.Get(context.Background(), u); !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive get", err)
	}
	for _, zone := range []string{"Asia/Almaty", "Europe/Berlin", "UTC"} {
		if !ValidTimezone(zone) {
			t.Fatal("valid timezone rejected", zone)
		}
	}
}
