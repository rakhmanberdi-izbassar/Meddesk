package users

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
	"net/http"
)

type AuthorizationGuard interface {
	Protect(http.Handler, ...string) http.Handler
}
type HTTPService interface {
	List(context.Context, auth.User, Filter) (Page, error)
	Create(context.Context, auth.User, CreateInput) (auth.User, error)
	Update(context.Context, auth.User, string, UpdateInput) (auth.User, error)
	Profile(context.Context, auth.User, ProfileInput) (auth.User, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(service HTTPService, guard AuthorizationGuard) *Handler {
	return &Handler{service, guard}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/users", h.guard.Protect(http.HandlerFunc(h.list), auth.RoleAdmin))
	mux.Handle("POST /api/v1/users", h.guard.Protect(http.HandlerFunc(h.create), auth.RoleAdmin))
	mux.Handle("PATCH /api/v1/users/{id}", h.guard.Protect(http.HandlerFunc(h.update), auth.RoleAdmin))
	mux.Handle("PATCH /api/v1/profile", h.guard.Protect(http.HandlerFunc(h.profile)))
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	p, err := httpx.ParsePagination(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.List(r.Context(), actor, Filter{Search: p.Query, Role: p.Role, Active: p.Active, Limit: int(p.Limit), Offset: int(p.Offset)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, result)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	var input CreateInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.Create(r.Context(), actor, input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 201, map[string]auth.User{"user": result})
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	var input UpdateInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.Update(r.Context(), actor, r.PathValue("id"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string]auth.User{"user": result})
}
func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	var input ProfileInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.Profile(r.Context(), actor, input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string]auth.User{"user": result})
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		response.Error(w, r, 400, "invalid_input", "Check the account fields and password length (12–72 bytes)")
	case errors.Is(err, ErrForbidden):
		response.Error(w, r, 403, "forbidden", "This operation is not permitted; administrators cannot demote or deactivate themselves")
	case errors.Is(err, ErrNotFound):
		response.Error(w, r, 404, "not_found", "User or department not found")
	case errors.Is(err, ErrConflict):
		response.Error(w, r, 409, "conflict", "An account with this email already exists")
	case errors.Is(err, ErrInactiveDepartment):
		response.Error(w, r, 409, "inactive_department", "Active users must belong to an active department")
	case errors.Is(err, ErrLastAdministrator):
		response.Error(w, r, 409, "last_administrator", "At least one active administrator is required")
	default:
		response.Error(w, r, 500, "internal_error", "Unable to complete the account operation")
	}
}
