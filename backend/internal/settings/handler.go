package settings

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
	Get(context.Context, auth.User) (Settings, error)
	Update(context.Context, auth.User, UpdateInput) (Settings, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(s HTTPService, g AuthorizationGuard) *Handler { return &Handler{s, g} }
func (h *Handler) RegisterRoutes(m *http.ServeMux) {
	m.Handle("GET /api/v1/settings", h.guard.Protect(http.HandlerFunc(h.get)))
	m.Handle("PATCH /api/v1/settings", h.guard.Protect(http.HandlerFunc(h.update), auth.RoleAdmin))
}
func actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
	}
	return u, ok
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, r, ErrInvalidInput)
		return
	}
	v, err := h.service.Get(r.Context(), u)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string]Settings{"settings": v})
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, r, ErrInvalidInput)
		return
	}
	var in UpdateInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	v, err := h.service.Update(r.Context(), u, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string]Settings{"settings": v})
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 500, "internal_error", "Unable to complete settings operation"
	switch {
	case errors.Is(err, ErrForbidden):
		status, code, message = 403, "forbidden", "This operation is not permitted"
	case errors.Is(err, ErrInvalidInput):
		status, code, message = 400, "invalid_input", "Check settings fields, timezone and version"
	case errors.Is(err, ErrVersionConflict):
		status, code, message = 409, "version_conflict", "Settings changed; reload before saving"
	}
	response.Error(w, r, status, code, message)
}
