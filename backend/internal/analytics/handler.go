package analytics

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
	"net/http"
)

type AuthorizationGuard interface {
	Protect(http.Handler, ...string) http.Handler
}
type HTTPService interface {
	Report(context.Context, auth.User, Filter) (Report, error)
	Filters(context.Context, auth.User) (Filters, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(s HTTPService, g AuthorizationGuard) *Handler { return &Handler{s, g} }
func (h *Handler) RegisterRoutes(m *http.ServeMux) {
	m.Handle("GET /api/v1/analytics", h.guard.Protect(http.HandlerFunc(h.report), auth.RoleEngineer, auth.RoleAdmin))
	m.Handle("GET /api/v1/analytics/filters", h.guard.Protect(http.HandlerFunc(h.filters), auth.RoleEngineer, auth.RoleAdmin))
}
func actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
	}
	return u, ok
}
func ValidateQuery(r *http.Request, keys ...string) bool {
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for k, v := range r.URL.Query() {
		if !allowed[k] || len(v) != 1 || v[0] == "" {
			return false
		}
	}
	return true
}
func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if !ValidateQuery(r, "from", "to", "timezone", "department_id", "category_id", "assignee_id") {
		writeError(w, r, ErrInvalidInput)
		return
	}
	q := r.URL.Query()
	out, err := h.service.Report(r.Context(), u, Filter{From: q.Get("from"), To: q.Get("to"), Timezone: q.Get("timezone"), DepartmentID: q.Get("department_id"), CategoryID: q.Get("category_id"), AssigneeID: q.Get("assignee_id")})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, out)
}
func (h *Handler) filters(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if !ValidateQuery(r) {
		writeError(w, r, ErrInvalidInput)
		return
	}
	out, err := h.service.Filters(r.Context(), u)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, out)
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 500, "internal_error", "Unable to load analytics"
	switch {
	case errors.Is(err, ErrInvalidInput):
		status, code, message = 400, "invalid_input", "Check dates, timezone and report filters (maximum 366 days)"
	case errors.Is(err, ErrForbidden):
		status, code, message = 403, "forbidden", "This operation is not permitted"
	}
	response.Error(w, r, status, code, message)
}
