package analytics

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"testing"
	"time"
)

const testActorID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

func staff() auth.User { return auth.User{ID: testActorID, Role: auth.RoleEngineer, IsActive: true} }

type fakeStore struct {
	reports, filters int
	now              time.Time
}

func (f *fakeStore) Report(_ context.Context, _ auth.User, _ Filter, now time.Time) (Report, error) {
	f.reports++
	f.now = now
	return Report{}, nil
}
func (f *fakeStore) Filters(context.Context, auth.User) (Filters, error) {
	f.filters++
	return Filters{}, nil
}
func TestCalendarPeriodsDefaultsBoundariesAndDST(t *testing.T) {
	now := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC)
	p, err := ResolvePeriod("", "", "Asia/Almaty", now)
	if err != nil || p.From != "2026-09-14" || p.To != "2026-10-13" || p.StartAt.Hour() != 19 || p.EndAt.Hour() != 19 {
		t.Fatalf("defaultlocal window: %+v %v", p, err)
	}
	for _, tc := range []struct {
		day   string
		hours time.Duration
	}{{"2026-03-08", 23}, {"2026-11-01", 25}} {
		p, err = ResolvePeriod(tc.day, tc.day, "America/New_York", now)
		if err != nil || p.EndAt.Sub(p.StartAt) != tc.hours*time.Hour {
			t.Fatalf("DST %+v: %+v %v", tc, p, err)
		}
	}
	p, err = ResolvePeriod("2024-01-01", "2024-12-31", "UTC", now)
	if err != nil || p.EndAt.Sub(p.StartAt) != 366*24*time.Hour {
		t.Fatalf("366calendar days: %+v %v", p, err)
	}
	for _, f := range []Filter{{From: "2026-02-30"}, {From: "2026-1-01"}, {From: "2026-10-13", To: "2026-10-12"}, {From: "2024-01-01", To: "2025-01-01"}, {From: "1969-12-31", To: "1970-01-01"}, {Timezone: "Local"}, {Timezone: "invalid/timezone"}} {
		zone := f.Timezone
		if zone == "" {
			zone = "UTC"
		}
		if _, err := ResolvePeriod(f.From, f.To, zone, now); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid period accepted: %+v %v", f, err)
		}
	}
}
func TestAnalyticsRolesFilterValidationAndSingleClock(t *testing.T) {
	repo := &fakeStore{}
	s := NewService(repo)
	calls := 0
	s.now = func() time.Time { calls++; return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()
	for _, f := range []Filter{{DepartmentID: "bad"}, {AssigneeID: "bad"}, {CategoryID: "bad"}, {Timezone: "bad"}, {From: "2026-01-01", To: "2027-01-02"}} {
		if _, err := s.Report(ctx, staff(), f); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid filter accepted", f, err)
		}
	}
	if repo.reports != 0 {
		t.Fatal("invalid filters reached store")
	}
	u := staff()
	u.Role = auth.RoleEmployee
	if _, err := s.Report(ctx, u, Filter{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("employee report authorized", err)
	}
	u = staff()
	u.IsActive = false
	if _, err := s.Filters(ctx, u); !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive filters authorized", err)
	}
	before := calls
	if _, err := s.Report(ctx, staff(), Filter{}); err != nil || calls-before != 1 || repo.reports != 1 {
		t.Fatal("request did not capture one clock", err, calls-before)
	}
	// No default endpoint is guessed in UTC before the configured timezone is read.
	if err := ValidateDates("2025-10-14", "", "", time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal("unspecifiedtimezone syntax rejected", err)
	}
}
