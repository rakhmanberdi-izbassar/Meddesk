package audit

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"testing"
	"time"
)

type fakeStore struct{ calls int }

func (f *fakeStore) List(context.Context, auth.User, Filter, time.Time) (Page, error) {
	f.calls++
	return Page{}, nil
}
func (f *fakeStore) Filters(context.Context, auth.User) (Filters, error) {
	f.calls++
	return Filters{}, nil
}
func TestAuditRoleAndFilterPolicy(t *testing.T) {
	repo := &fakeStore{}
	s := NewService(repo)
	ctx := context.Background()
	u := auth.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Role: auth.RoleAdmin, IsActive: true}
	cases := []Filter{{Limit: 0}, {Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, Offset: 100001}, {Limit: 20, ActorID: "bad"}, {Limit: 20, EntityID: "bad"}, {Limit: 20, Action: "bad/input"}, {Limit: 20, EntityType: "bad input"}, {Limit: 20, From: "2026-02-30"}, {Limit: 20, From: "2024-01-01", To: "2025-01-01"}, {Limit: 20, Timezone: "Local"}}
	for _, f := range cases {
		if _, err := s.List(ctx, u, f); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid filter accepted: %+v %v", f, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid filter reached database")
	}
	u.Role = auth.RoleEngineer
	if _, err := s.List(ctx, u, Filter{Limit: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatal("engineer audit allowed", err)
	}
	u.Role = auth.RoleAdmin
	u.IsActive = false
	if _, err := s.Filters(ctx, u); !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive audit allowed", err)
	}
	u.IsActive = true
	if _, err := s.List(ctx, u, Filter{Limit: 20, Action: "ticket.created", EntityType: "ticket"}); err != nil || repo.calls != 1 {
		t.Fatal("valid audit rejected", err)
	}
}
