package audit

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
	"net/http"
	"strconv"
)

type AuthorizationGuard interface {
	Protect(http.Handler, ...string) http.Handler
}
type HTTPService interface {
	List(context.Context, auth.User, Filter) (Page, error)
	Filters(context.Context, auth.User) (Filters, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(s HTTPService, g AuthorizationGuard) *Handler { return &Handler{s, g} }
func (h *Handler) RegisterRoutes(m *http.ServeMux) {
	m.Handle("GET /api/v1/admin/audit", h.guard.Protect(http.HandlerFunc(h.list), auth.RoleAdmin))
	m.Handle("GET /api/v1/admin/audit/filters", h.guard.Protect(http.HandlerFunc(h.filters), auth.RoleAdmin))
}
func actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
	}
	return u, ok
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if !analytics.ValidateQuery(r, "from", "to", "timezone", "actor_id", "action", "entity_type", "entity_id", "limit", "offset") {
		writeError(w, r, ErrInvalidInput)
		return
	}
	q := r.URL.Query()
	f := Filter{From: q.Get("from"), To: q.Get("to"), Timezone: q.Get("timezone"), ActorID: q.Get("actor_id"), Action: q.Get("action"), EntityType: q.Get("entity_type"), EntityID: q.Get("entity_id"), Limit: 20}
	var err error
	if v := q.Get("limit"); v != "" {
		f.Limit, err = strconv.Atoi(v)
		if err != nil {
			writeError(w, r, ErrInvalidInput)
			return
		}
	}
	if v := q.Get("offset"); v != "" {
		f.Offset, err = strconv.Atoi(v)
		if err != nil {
			writeError(w, r, ErrInvalidInput)
			return
		}
	}
	out, err := h.service.List(r.Context(), u, f)
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
	if !analytics.ValidateQuery(r) {
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
	status, code, message := 500, "internal_error", "Unable to load audit events"
	switch {
	case errors.Is(err, ErrInvalidInput):
		status, code, message = 400, "invalid_input", "Check dates, timezone, identifiers and pagination"
	case errors.Is(err, ErrForbidden):
		status, code, message = 403, "forbidden", "This operation is not permitted"
	}
	response.Error(w, r, status, code, message)
}
