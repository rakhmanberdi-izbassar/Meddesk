package departments

import (
	"context"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"strings"
)

type Service struct{ repo Store }

func NewService(repo Store) *Service { return &Service{repo} }
func isAdmin(actor auth.User) bool   { return actor.IsActive && actor.Role == auth.RoleAdmin }
func validName(name string) bool     { return httpx.ValidText(name, 150) }
func (s *Service) List(ctx context.Context, actor auth.User, f Filter) (Page, error) {
	if !isAdmin(actor) {
		return Page{}, ErrForbidden
	}
	f.Search = strings.TrimSpace(f.Search)
	if (f.Search != "" && !httpx.ValidText(f.Search, 100)) || f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 {
		return Page{}, ErrInvalidInput
	}
	return s.repo.List(ctx, f)
}
func (s *Service) Create(ctx context.Context, actor auth.User, input CreateInput) (Department, error) {
	if !isAdmin(actor) {
		return Department{}, ErrForbidden
	}
	name := strings.TrimSpace(input.Name)
	if !validName(name) {
		return Department{}, ErrInvalidInput
	}
	var result Department
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		if err := tx.Lock(ctx); err != nil {
			return err
		}
		if err := authorizeMutation(ctx, tx, actor); err != nil {
			return err
		}
		var err error
		result, err = tx.Create(ctx, name)
		if err != nil {
			return err
		}
		return tx.Audit(ctx, actor.ID, "department.created", result.ID)
	})
	return result, err
}
func (s *Service) Update(ctx context.Context, actor auth.User, id string, input UpdateInput) (Department, error) {
	if !isAdmin(actor) {
		return Department{}, ErrForbidden
	}
	if _, err := parseUUID(id); err != nil {
		return Department{}, err
	}
	name := strings.TrimSpace(input.Name)
	if !validName(name) || input.IsActive == nil {
		return Department{}, ErrInvalidInput
	}
	var result Department
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		if err := tx.Lock(ctx); err != nil {
			return err
		}
		if err := authorizeMutation(ctx, tx, actor); err != nil {
			return err
		}
		current, err := tx.Get(ctx, id)
		if err != nil {
			return err
		}
		if current.IsActive && !*input.IsActive {
			count, err := tx.ActiveUsers(ctx, id)
			if err != nil {
				return err
			}
			if count > 0 {
				return ErrHasActiveUsers
			}
		}
		result, err = tx.Update(ctx, id, name, *input.IsActive)
		if err != nil {
			return err
		}
		return tx.Audit(ctx, actor.ID, "department.updated", id)
	})
	return result, err
}
func authorizeMutation(ctx context.Context, tx MutationStore, actor auth.User) error {
	current, err := tx.Actor(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !isAdmin(current) {
		return ErrForbidden
	}
	return nil
}
