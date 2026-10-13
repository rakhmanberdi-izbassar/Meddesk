package tickets

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type AuthorizationGuard interface {
	Protect(http.Handler, ...string) http.Handler
}
type HTTPService interface {
	List(context.Context, auth.User, Filter) (Page, error)
	Export(context.Context, auth.User, ExportInput) (ExportFile, error)
	Get(context.Context, auth.User, string) (Detail, error)
	Create(context.Context, auth.User, CreateInput) (Ticket, error)
	Update(context.Context, auth.User, string, UpdateInput) (Ticket, error)
	Comment(context.Context, auth.User, string, CommentInput) (CommentResult, error)
	Attach(context.Context, auth.User, string, AttachmentInput) (Ticket, error)
	Confirm(context.Context, auth.User, string, VersionInput) (Ticket, error)
	Reopen(context.Context, auth.User, string, ReopenInput) (Ticket, error)
	Convert(context.Context, auth.User, string, ArticleInput) (knowledge.Article, error)
	Assignees(context.Context, auth.User) ([]Assignee, error)
	Departments(context.Context, auth.User) ([]Department, error)
	Suggestions(context.Context, auth.User, string, string) (knowledge.ArticlePage, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(service HTTPService, guard AuthorizationGuard) *Handler {
	return &Handler{service, guard}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	const base = "/api/v1/tickets"
	mux.Handle("GET "+base, h.guard.Protect(http.HandlerFunc(h.list)))
	mux.Handle("GET "+base+"/export", h.guard.Protect(http.HandlerFunc(h.export), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("GET "+base+"/{id}", h.guard.Protect(http.HandlerFunc(h.get)))
	mux.Handle("GET "+base+"/suggestions", h.guard.Protect(http.HandlerFunc(h.suggestions)))
	mux.Handle("GET "+base+"/assignees", h.guard.Protect(http.HandlerFunc(h.assignees), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("GET "+base+"/departments", h.guard.Protect(http.HandlerFunc(h.departments), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("POST "+base, h.guard.Protect(http.HandlerFunc(h.create)))
	mux.Handle("PATCH "+base+"/{id}", h.guard.Protect(http.HandlerFunc(h.update), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("POST "+base+"/{id}/comments", h.guard.Protect(http.HandlerFunc(h.comment)))
	mux.Handle("POST "+base+"/{id}/attachments", h.guard.Protect(http.HandlerFunc(h.attach)))
	mux.Handle("POST "+base+"/{id}/confirm", h.guard.Protect(http.HandlerFunc(h.confirm)))
	mux.Handle("POST "+base+"/{id}/reopen", h.guard.Protect(http.HandlerFunc(h.reopen), auth.RoleAdmin))
	mux.Handle("POST "+base+"/{id}/article", h.guard.Protect(http.HandlerFunc(h.convert), auth.RoleEngineer, auth.RoleAdmin))
}
func actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
	}
	return u, ok
}
func filters(r *http.Request, keys ...string) error {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return ErrInvalidInput
	}
	allowed := map[string]bool{}
	for _, key := range keys {
		allowed[key] = true
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 || values[0] == "" {
			return ErrInvalidInput
		}
	}
	return nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if err := filters(r, "q", "status", "priority", "assignee_id", "department_id", "category_id", "mine", "limit", "offset", "from", "to", "timezone"); err != nil {
		writeError(w, r, err)
		return
	}
	q := r.URL.Query()
	f, err := queryFilter(q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	f.Limit = 20
	if value := q.Get("limit"); value != "" {
		f.Limit, err = strconv.Atoi(value)
		if err != nil {
			writeError(w, r, ErrInvalidInput)
			return
		}
	}
	if value := q.Get("offset"); value != "" {
		f.Offset, err = strconv.Atoi(value)
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
func queryFilter(q url.Values) (Filter, error) {
	f := Filter{Search: q.Get("q"), Status: q.Get("status"), Priority: q.Get("priority"), AssigneeID: q.Get("assignee_id"), DepartmentID: q.Get("department_id"), CategoryID: q.Get("category_id"), From: q.Get("from"), To: q.Get("to"), Timezone: q.Get("timezone")}
	if value := q.Get("mine"); value != "" {
		if value != "true" && value != "false" {
			return Filter{}, ErrInvalidInput
		}
		f.Mine = value == "true"
	}
	return f, nil
}
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if err := filters(r, "q", "status", "priority", "assignee_id", "department_id", "category_id", "mine", "from", "to", "timezone", "format", "locale"); err != nil {
		writeError(w, r, err)
		return
	}
	q := r.URL.Query()
	f, err := queryFilter(q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	file, err := h.service.Export(r.Context(), u, ExportInput{Filter: f, Format: q.Get("format"), Locale: q.Get("locale")})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+file.Filename+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Cookie")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	out, err := h.service.Get(r.Context(), u, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, out)
}
func (h *Handler) suggestions(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if err := filters(r, "q", "category_id"); err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.service.Suggestions(r.Context(), u, r.URL.Query().Get("q"), r.URL.Query().Get("category_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, out)
}
func (h *Handler) assignees(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if err := filters(r); err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.service.Assignees(r.Context(), u)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string][]Assignee{"items": out})
}
func (h *Handler) departments(w http.ResponseWriter, r *http.Request) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	if err := filters(r); err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.service.Departments(r.Context(), u)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string][]Department{"items": out})
}
func decode[T any](w http.ResponseWriter, r *http.Request, fn func(auth.User, T) (any, error), status int) {
	u, ok := actor(w, r)
	if !ok {
		return
	}
	var in T
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := fn(u, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, status, out)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in CreateInput) (any, error) {
		t, err := h.service.Create(r.Context(), u, in)
		return map[string]Ticket{"ticket": t}, err
	}, 201)
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in UpdateInput) (any, error) {
		t, err := h.service.Update(r.Context(), u, r.PathValue("id"), in)
		return map[string]Ticket{"ticket": t}, err
	}, 200)
}
func (h *Handler) comment(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in CommentInput) (any, error) {
		return h.service.Comment(r.Context(), u, r.PathValue("id"), in)
	}, 200)
}
func (h *Handler) attach(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in AttachmentInput) (any, error) {
		t, err := h.service.Attach(r.Context(), u, r.PathValue("id"), in)
		return map[string]Ticket{"ticket": t}, err
	}, 200)
}
func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in VersionInput) (any, error) {
		t, err := h.service.Confirm(r.Context(), u, r.PathValue("id"), in)
		return map[string]Ticket{"ticket": t}, err
	}, 200)
}
func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in ReopenInput) (any, error) {
		t, err := h.service.Reopen(r.Context(), u, r.PathValue("id"), in)
		return map[string]Ticket{"ticket": t}, err
	}, 200)
}
func (h *Handler) convert(w http.ResponseWriter, r *http.Request) {
	decode(w, r, func(u auth.User, in ArticleInput) (any, error) {
		a, err := h.service.Convert(r.Context(), u, r.PathValue("id"), in)
		return map[string]knowledge.Article{"article": a}, err
	}, 201)
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 500, "internal_error", "Unable to complete the ticket operation"
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, knowledge.ErrInvalidInput):
		status, code, message = 400, "invalid_input", "Check ticket fields, identifiers and filters"
	case errors.Is(err, ErrForbidden), errors.Is(err, knowledge.ErrForbidden):
		status, code, message = 403, "forbidden", "This operation is not permitted"
	case errors.Is(err, ErrNotFound):
		status, code, message = 404, "not_found", "Ticket resource not found"
	case errors.Is(err, ErrVersionConflict):
		status, code, message = 409, "version_conflict", "Ticket changed; reload before saving"
	case errors.Is(err, ErrExportLimit):
		status, code, message = 409, "export_limit", "Export exceeds the 5000-ticket or 20 MiB limit; narrow the date range or filters"
	case errors.Is(err, ErrClosed):
		status, code, message = 409, "ticket_closed", "Closed tickets require administrator reopening"
	case errors.Is(err, ErrTransition):
		status, code, message = 409, "invalid_transition", "This ticket status transition is not permitted"
	case errors.Is(err, ErrInactiveCategory):
		status, code, message = 409, "inactive_category", "Choose an active category"
	case errors.Is(err, ErrAlreadyConverted):
		status, code, message = 409, "already_converted", "This ticket already has a converted draft"
	case errors.Is(err, ErrConflict):
		status, code, message = 409, "conflict", "A resource conflicts with this change"
	}
	response.Error(w, r, status, code, message)
}
