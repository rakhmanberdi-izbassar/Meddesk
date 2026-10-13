package audit

import (
	"context"
	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"regexp"
	"time"
)

type Service struct {
	repo Store
	now  func() time.Time
}

func NewService(r Store) *Service { return &Service{r, time.Now} }
func isAdmin(u auth.User) bool {
	return u.IsActive && u.Role == auth.RoleAdmin && httpx.ValidUUID(u.ID)
}

var routingName = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)

func (s *Service) List(ctx context.Context, u auth.User, f Filter) (Page, error) {
	if !isAdmin(u) {
		return Page{}, ErrForbidden
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 {
		return Page{}, ErrInvalidInput
	}
	for _, id := range []string{f.ActorID, f.EntityID} {
		if id != "" && !httpx.ValidUUID(id) {
			return Page{}, ErrInvalidInput
		}
	}
	for _, name := range []string{f.Action, f.EntityType} {
		if name != "" && !routingName.MatchString(name) {
			return Page{}, ErrInvalidInput
		}
	}
	now := s.now()
	if err := analytics.ValidateDates(f.From, f.To, f.Timezone, now); err != nil {
		return Page{}, ErrInvalidInput
	}
	return s.repo.List(ctx, u, f, now)
}
func (s *Service) Filters(ctx context.Context, u auth.User) (Filters, error) {
	if !isAdmin(u) {
		return Filters{}, ErrForbidden
	}
	return s.repo.Filters(ctx, u)
}
