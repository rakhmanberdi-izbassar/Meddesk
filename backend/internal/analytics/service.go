package analytics

import (
	"context"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"github.com/meddesk/meddesk/backend/internal/settings"
	"time"
)

type Service struct {
	repo Store
	now  func() time.Time
}

func NewService(r Store) *Service { return &Service{r, time.Now} }
func IsStaff(u auth.User) bool {
	return u.IsActive && httpx.ValidUUID(u.ID) && (u.Role == auth.RoleEngineer || u.Role == auth.RoleAdmin)
}

// ResolvePeriod converts inclusive local calendar dates into an exclusive instant
// boundary using calendar arithmetic, so daylight-saving days remain complete.
func ResolvePeriod(from, to, zone string, now time.Time) (Period, error) {
	if !settings.ValidTimezone(zone) {
		return Period{}, ErrInvalidInput
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Period{}, ErrInvalidInput
	}
	current := now.In(loc)
	end := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, loc)
	if to != "" {
		end, err = time.ParseInLocation("2006-01-02", to, loc)
		if err != nil || end.Format("2006-01-02") != to {
			return Period{}, ErrInvalidInput
		}
	}
	start := end.AddDate(0, 0, -29)
	if from != "" {
		start, err = time.ParseInLocation("2006-01-02", from, loc)
		if err != nil || start.Format("2006-01-02") != from {
			return Period{}, ErrInvalidInput
		}
	}
	if start.Year() < 1970 || end.Year() > 9998 || start.After(end) || end.After(start.AddDate(0, 0, 365)) {
		return Period{}, ErrInvalidInput
	}
	return Period{From: start.Format("2006-01-02"), To: end.Format("2006-01-02"), Timezone: zone, StartAt: start.UTC(), EndAt: end.AddDate(0, 0, 1).UTC()}, nil
}

// ValidateDates checks syntax before reading the configured reporting timezone.
// Bounds involving a default endpoint are resolved only inside the snapshot.
func ValidateDates(from, to, zone string, now time.Time) error {
	if zone != "" {
		_, err := ResolvePeriod(from, to, zone, now)
		return err
	}
	for _, value := range []string{from, to} {
		if value == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", value)
		if err != nil || day.Format("2006-01-02") != value || day.Year() < 1970 || day.Year() > 9998 {
			return ErrInvalidInput
		}
	}
	if from != "" && to != "" {
		_, err := ResolvePeriod(from, to, "UTC", now)
		return err
	}
	return nil
}
func (s *Service) Report(ctx context.Context, u auth.User, f Filter) (Report, error) {
	if !IsStaff(u) {
		return Report{}, ErrForbidden
	}
	for _, id := range []string{f.DepartmentID, f.CategoryID, f.AssigneeID} {
		if id != "" && !httpx.ValidUUID(id) {
			return Report{}, ErrInvalidInput
		}
	}
	now := s.now()
	if err := ValidateDates(f.From, f.To, f.Timezone, now); err != nil {
		return Report{}, err
	}
	return s.repo.Report(ctx, u, f, now)
}
func (s *Service) Filters(ctx context.Context, u auth.User) (Filters, error) {
	if !IsStaff(u) {
		return Filters{}, ErrForbidden
	}
	return s.repo.Filters(ctx, u)
}
