package departments

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
	Create(context.Context, auth.User, CreateInput) (Department, error)
	Update(context.Context, auth.User, string, UpdateInput) (Department, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(service HTTPService, guard AuthorizationGuard) *Handler {
	return &Handler{service, guard}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/departments", h.guard.Protect(http.HandlerFunc(h.list), auth.RoleAdmin))
	mux.Handle("POST /api/v1/departments", h.guard.Protect(http.HandlerFunc(h.create), auth.RoleAdmin))
	mux.Handle("PATCH /api/v1/departments/{id}", h.guard.Protect(http.HandlerFunc(h.update), auth.RoleAdmin))
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
	if p.Role != "" {
		httpx.WriteError(w, r, &httpx.InputError{Status: 400, Code: "invalid_filter", Message: "Role does not apply to departments"})
		return
	}
	result, err := h.service.List(r.Context(), actor, Filter{Search: p.Query, Active: p.Active, Limit: int(p.Limit), Offset: int(p.Offset)})
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
	response.JSON(w, 201, map[string]Department{"department": result})
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
	response.JSON(w, 200, map[string]Department{"department": result})
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		response.Error(w, r, 400, "invalid_input", "Department name and active status are required")
	case errors.Is(err, ErrForbidden):
		response.Error(w, r, 403, "forbidden", "Administrator access is required")
	case errors.Is(err, ErrNotFound):
		response.Error(w, r, 404, "not_found", "Department not found")
	case errors.Is(err, ErrConflict):
		response.Error(w, r, 409, "conflict", "A department with this name already exists")
	case errors.Is(err, ErrHasActiveUsers):
		response.Error(w, r, 409, "department_has_active_users", "Move or deactivate active users before deactivating their department")
	default:
		response.Error(w, r, 500, "internal_error", "Unable to complete the department operation")
	}
}
