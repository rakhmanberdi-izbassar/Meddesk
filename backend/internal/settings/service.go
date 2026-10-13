package settings

import (
	"context"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"strings"
	"time"
	_ "time/tzdata"
)

type Service struct{ repo Store }

func NewService(repo Store) *Service { return &Service{repo} }
func active(u auth.User) bool {
	return u.IsActive && httpx.ValidUUID(u.ID) && (u.Role == auth.RoleAdmin || u.Role == auth.RoleEngineer || u.Role == auth.RoleEmployee)
}
func ValidTimezone(s string) bool {
	if s == "" || len(s) > 100 || s == "Local" || strings.HasPrefix(s, "/") || strings.Contains(s, "..") {
		return false
	}
	_, err := time.LoadLocation(s)
	return err == nil
}
func (s *Service) Get(ctx context.Context, u auth.User) (Settings, error) {
	if !active(u) {
		return Settings{}, ErrForbidden
	}
	return s.repo.Get(ctx, u)
}
func (s *Service) Update(ctx context.Context, u auth.User, in UpdateInput) (Settings, error) {
	if !active(u) || u.Role != auth.RoleAdmin {
		return Settings{}, ErrForbidden
	}
	if in.Version < 1 || (in.ClinicName == nil && in.DefaultTicketPriority == nil && in.ReportingTimezone == nil) {
		return Settings{}, ErrInvalidInput
	}
	if in.ClinicName != nil {
		name := strings.TrimSpace(*in.ClinicName)
		if !httpx.ValidText(name, 150) {
			return Settings{}, ErrInvalidInput
		}
		in.ClinicName = &name
	}
	if in.DefaultTicketPriority != nil {
		switch *in.DefaultTicketPriority {
		case "LOW", "MEDIUM", "HIGH", "CRITICAL":
		default:
			return Settings{}, ErrInvalidInput
		}
	}
	if in.ReportingTimezone != nil && !ValidTimezone(*in.ReportingTimezone) {
		return Settings{}, ErrInvalidInput
	}
	var out Settings
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		if err := tx.Lock(ctx); err != nil {
			return err
		}
		actor, err := tx.Actor(ctx, u.ID)
		if err != nil {
			return err
		}
		if !active(actor) || actor.Role != auth.RoleAdmin {
			return ErrForbidden
		}
		current, err := tx.Get(ctx)
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return ErrVersionConflict
		}
		changed := make([]string, 0, 3)
		if in.ClinicName != nil {
			if current.ClinicName != *in.ClinicName {
				changed = append(changed, "clinic_name")
			}
			current.ClinicName = *in.ClinicName
		}
		if in.DefaultTicketPriority != nil {
			if current.DefaultTicketPriority != *in.DefaultTicketPriority {
				changed = append(changed, "default_ticket_priority")
			}
			current.DefaultTicketPriority = *in.DefaultTicketPriority
		}
		if in.ReportingTimezone != nil {
			if current.ReportingTimezone != *in.ReportingTimezone {
				changed = append(changed, "reporting_timezone")
			}
			current.ReportingTimezone = *in.ReportingTimezone
		}
		out, err = tx.Update(ctx, current)
		if err != nil {
			return err
		}
		return tx.Audit(ctx, actor.ID, out.Version, changed)
	})
	return out, err
}
