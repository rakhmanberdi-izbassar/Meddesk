package users

import (
	"context"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"strings"
)

type Service struct{ repo Store }

func NewService(repo Store) *Service { return &Service{repo} }
func validRole(role string) bool {
	return role == auth.RoleEmployee || role == auth.RoleEngineer || role == auth.RoleAdmin
}
func validName(name string) bool   { return httpx.ValidText(name, 150) }
func isAdmin(actor auth.User) bool { return actor.IsActive && actor.Role == auth.RoleAdmin }
func (s *Service) List(ctx context.Context, actor auth.User, f Filter) (Page, error) {
	if !isAdmin(actor) {
		return Page{}, ErrForbidden
	}
	f.Search = strings.TrimSpace(f.Search)
	if (f.Search != "" && !httpx.ValidText(f.Search, 100)) || (!validRole(f.Role) && f.Role != "") || f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 {
		return Page{}, ErrInvalidInput
	}
	return s.repo.List(ctx, f)
}
func (s *Service) Create(ctx context.Context, actor auth.User, input CreateInput) (auth.User, error) {
	if !isAdmin(actor) {
		return auth.User{}, ErrForbidden
	}
	email, err := auth.NormalizeEmail(input.Email)
	if err != nil {
		return auth.User{}, ErrInvalidInput
	}
	name := strings.TrimSpace(input.DisplayName)
	if !validName(name) || !validRole(input.Role) {
		return auth.User{}, ErrInvalidInput
	}
	if _, err := parseUUID(input.DepartmentID); err != nil {
		return auth.User{}, err
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return auth.User{}, ErrInvalidInput
	}
	var user auth.User
	err = s.repo.Transaction(ctx, func(tx MutationStore) error {
		if err := tx.Lock(ctx); err != nil {
			return err
		}
		if err := authorizeMutation(ctx, tx, actor); err != nil {
			return err
		}
		active, err := tx.ActiveDepartment(ctx, input.DepartmentID)
		if err != nil {
			return err
		}
		if !active {
			return ErrInactiveDepartment
		}
		user, err = tx.Create(ctx, AccountMutation{Email: email, DisplayName: name, Role: input.Role, DepartmentID: input.DepartmentID, IsActive: true, PasswordHash: &hash})
		if err != nil {
			return err
		}
		return tx.Audit(ctx, actor.ID, "user.created", user.ID, map[string]any{"role": user.Role, "department_id": user.DepartmentID})
	})
	return user, err
}
func (s *Service) Update(ctx context.Context, actor auth.User, id string, input UpdateInput) (auth.User, error) {
	if !isAdmin(actor) {
		return auth.User{}, ErrForbidden
	}
	if _, err := parseUUID(id); err != nil {
		return auth.User{}, err
	}
	if _, err := parseUUID(input.DepartmentID); err != nil {
		return auth.User{}, err
	}
	email, err := auth.NormalizeEmail(input.Email)
	if err != nil {
		return auth.User{}, ErrInvalidInput
	}
	name := strings.TrimSpace(input.DisplayName)
	if !validName(name) || !validRole(input.Role) || input.IsActive == nil {
		return auth.User{}, ErrInvalidInput
	}
	id = strings.ToLower(id)
	if id == actor.ID && (!*input.IsActive || input.Role != auth.RoleAdmin) {
		return auth.User{}, ErrForbidden
	}
	var hash *string
	if input.Password != nil {
		value, err := auth.HashPassword(*input.Password)
		if err != nil {
			return auth.User{}, ErrInvalidInput
		}
		hash = &value
	}
	var user auth.User
	err = s.repo.Transaction(ctx, func(tx MutationStore) error {
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
		if current.IsActive && current.Role == auth.RoleAdmin && (!*input.IsActive || input.Role != auth.RoleAdmin) {
			count, err := tx.ActiveAdministrators(ctx)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdministrator
			}
		}
		active, err := tx.ActiveDepartment(ctx, input.DepartmentID)
		if err != nil {
			return err
		}
		if *input.IsActive && !active {
			return ErrInactiveDepartment
		}
		user, err = tx.Update(ctx, id, AccountMutation{Email: email, DisplayName: name, Role: input.Role, DepartmentID: input.DepartmentID, IsActive: *input.IsActive, PasswordHash: hash})
		if err != nil {
			return err
		}
		revoke := hash != nil || current.Role != user.Role || current.IsActive != user.IsActive || current.Email != user.Email
		if revoke {
			if err := tx.RevokeSessions(ctx, id); err != nil {
				return err
			}
		}
		return tx.Audit(ctx, actor.ID, "user.updated", id, map[string]any{"role": user.Role, "is_active": user.IsActive, "department_id": user.DepartmentID, "password_reset": hash != nil, "sessions_revoked": revoke})
	})
	return user, err
}
func (s *Service) Profile(ctx context.Context, actor auth.User, input ProfileInput) (auth.User, error) {
	if !actor.IsActive {
		return auth.User{}, ErrForbidden
	}
	name := strings.TrimSpace(input.DisplayName)
	if !validName(name) {
		return auth.User{}, ErrInvalidInput
	}
	var user auth.User
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		current, err := tx.Get(ctx, actor.ID)
		if err != nil {
			return err
		}
		if !current.IsActive {
			return ErrForbidden
		}
		user, err = tx.Profile(ctx, actor.ID, name)
		if err != nil {
			return err
		}
		return tx.Audit(ctx, actor.ID, "profile.updated", actor.ID, map[string]any{})
	})
	return user, err
}
func authorizeMutation(ctx context.Context, tx MutationStore, actor auth.User) error {
	current, err := tx.Get(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !isAdmin(current) {
		return ErrForbidden
	}
	return nil
}
