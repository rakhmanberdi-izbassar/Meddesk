package school

import (
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
	"net/http"
	"strconv"
)

type Guard interface {
	Protect(http.Handler, ...string) http.Handler
}
type Handler struct {
	service *Service
	guard   Guard
}

func NewHandler(s *Service, g Guard) *Handler { return &Handler{s, g} }
func (h *Handler) RegisterRoutes(m *http.ServeMux) {
	for _, route := range []string{"GET /api/v1/school/access", "GET /api/v1/school/assets", "GET /api/v1/school/assets/{id}", "POST /api/v1/school/assets", "PATCH /api/v1/school/assets/{id}", "GET /api/v1/school/tickets", "POST /api/v1/school/tickets", "PATCH /api/v1/school/tickets/{id}"} {
		m.Handle(route, h.guard.Protect(http.HandlerFunc(h.serve)))
	}
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 500, "internal_error"
	switch {
	case errors.Is(err, ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, ErrConflict):
		status, code = 409, "version_conflict"
	case errors.Is(err, ErrNotFound):
		status, code = 404, "not_found"
	}
	response.Error(w, r, status, code, "Unable to complete school operation")
}
func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	ctx := r.Context()
	path := r.URL.Path
	if path == "/api/v1/school/access" {
		if len(r.URL.Query()) > 0 {
			fail(w, r, ErrInvalid)
			return
		}
		allowed, err := h.service.Access(ctx, u.ID)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, 200, map[string]bool{"allowed": allowed})
		return
	}
	if r.Method == "GET" && r.PathValue("id") != "" {
		if len(r.URL.Query()) > 0 {
			fail(w, r, ErrInvalid)
			return
		}
		out, err := h.service.Asset(ctx, u.ID, r.PathValue("id"))
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, 200, out)
		return
	}
	var out any
	var err error
	status := 200
	if r.Method == "GET" {
		limit, offset := int64(20), int64(0)
		for key, values := range r.URL.Query() {
			if len(values) != 1 || !oneOf(key, "limit", "offset", "status", "asset_id", "q") || key == "q" && path != "/api/v1/school/assets" || (key == "status" || key == "asset_id") && path != "/api/v1/school/tickets" {
				fail(w, r, ErrInvalid)
				return
			}
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			limit, err = strconv.ParseInt(v, 10, 32)
			if err != nil {
				fail(w, r, ErrInvalid)
				return
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			offset, err = strconv.ParseInt(v, 10, 32)
			if err != nil {
				fail(w, r, ErrInvalid)
				return
			}
		}
		if path == "/api/v1/school/assets" {
			out, err = h.service.Assets(ctx, u.ID, r.URL.Query().Get("q"), int32(limit), int32(offset))
		} else {
			out, err = h.service.Tickets(ctx, u.ID, r.URL.Query().Get("status"), r.URL.Query().Get("asset_id"), int32(limit), int32(offset))
		}
	} else {
		if len(r.URL.Query()) > 0 {
			fail(w, r, ErrInvalid)
			return
		}
		switch {
		case path == "/api/v1/school/assets" || r.Pattern == "PATCH /api/v1/school/assets/{id}":
			var in AssetInput
			if e := httpx.Decode(w, r, &in); e != nil {
				httpx.WriteError(w, r, e)
				return
			}
			out, err = h.service.SaveAsset(ctx, u.ID, r.PathValue("id"), in)
		case r.Method == "POST":
			var in TicketInput
			if e := httpx.Decode(w, r, &in); e != nil {
				httpx.WriteError(w, r, e)
				return
			}
			out, err = h.service.CreateTicket(ctx, u.ID, in)
		default:
			var in UpdateInput
			if e := httpx.Decode(w, r, &in); e != nil {
				httpx.WriteError(w, r, e)
				return
			}
			out, err = h.service.UpdateTicket(ctx, u.ID, r.PathValue("id"), in)
		}
		if r.Method == "POST" {
			status = 201
		}
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, status, out)
}
