package knowledge

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type AuthorizationGuard interface {
	Protect(http.Handler, ...string) http.Handler
}
type HTTPService interface {
	List(context.Context, auth.User, Filter) (ArticlePage, error)
	Get(context.Context, auth.User, string) (Article, error)
	GetBySlug(context.Context, auth.User, string) (Article, error)
	Save(context.Context, auth.User, string, ArticleInput) (Article, error)
	Delete(context.Context, auth.User, string, int32) error
	Categories(context.Context, auth.User, CategoryFilter) (CategoryPage, error)
	SaveCategory(context.Context, auth.User, string, CategoryInput) (Category, error)
	Tags(context.Context, auth.User, string, int) ([]string, error)
	RecordView(context.Context, auth.User, string) (ViewResult, error)
	Feedback(context.Context, auth.User, string, FeedbackInput) (FeedbackResult, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(service HTTPService, guard AuthorizationGuard) *Handler {
	return &Handler{service, guard}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	const base = "/api/v1/knowledge"
	mux.Handle("GET "+base+"/articles", h.guard.Protect(http.HandlerFunc(h.list)))
	mux.Handle("GET "+base+"/articles/{id}", h.guard.Protect(http.HandlerFunc(h.get)))
	mux.Handle("GET "+base+"/articles/by-slug/{slug}", h.guard.Protect(http.HandlerFunc(h.getSlug)))
	mux.Handle("POST "+base+"/articles", h.guard.Protect(http.HandlerFunc(h.save), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("PATCH "+base+"/articles/{id}", h.guard.Protect(http.HandlerFunc(h.save), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("DELETE "+base+"/articles/{id}", h.guard.Protect(http.HandlerFunc(h.delete), auth.RoleEngineer, auth.RoleAdmin))
	mux.Handle("POST "+base+"/articles/{id}/views", h.guard.Protect(http.HandlerFunc(h.view)))
	mux.Handle("POST "+base+"/articles/{id}/feedback", h.guard.Protect(http.HandlerFunc(h.feedback)))
	mux.Handle("GET "+base+"/categories", h.guard.Protect(http.HandlerFunc(h.categories)))
	mux.Handle("POST "+base+"/categories", h.guard.Protect(http.HandlerFunc(h.saveCategory), auth.RoleAdmin))
	mux.Handle("PATCH "+base+"/categories/{id}", h.guard.Protect(http.HandlerFunc(h.saveCategory), auth.RoleAdmin))
	mux.Handle("GET "+base+"/tags", h.guard.Protect(http.HandlerFunc(h.tags)))
}
func actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
	}
	return user, ok
}
func allowedFilters(r *http.Request, keys ...string) error {
	allowed := map[string]bool{}
	for _, key := range keys {
		allowed[key] = true
	}
	for key, values := range r.URL.Query() {
		if !allowed[key] || len(values) != 1 || values[0] == "" {
			return ErrInvalidInput
		}
	}
	return nil
}
func page(r *http.Request) (int, int, error) {
	limit := 20
	offset := 0
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, ErrInvalidInput
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, ErrInvalidInput
		}
	}
	if !validPage(limit, offset) {
		return 0, 0, ErrInvalidInput
	}
	return limit, offset, nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	if err := allowedFilters(r, "q", "category_id", "tag", "published", "limit", "offset"); err != nil {
		writeError(w, r, err)
		return
	}
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	yes := true
	f := Filter{Search: r.URL.Query().Get("q"), CategoryID: r.URL.Query().Get("category_id"), Tag: r.URL.Query().Get("tag"), Published: &yes, PublicOnly: true, Limit: limit, Offset: offset}
	switch r.URL.Query().Get("published") {
	case "", "true":
	case "false":
		no := false
		f.Published = &no
		f.PublicOnly = false
	case "all":
		f.Published = nil
		f.PublicOnly = false
	default:
		writeError(w, r, ErrInvalidInput)
		return
	}
	if !staff(user) && !f.PublicOnly {
		writeError(w, r, ErrForbidden)
		return
	}
	result, err := h.service.List(r.Context(), user, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, result)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string]Article{"article": result})
}
func (h *Handler) getSlug(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	result, err := h.service.GetBySlug(r.Context(), user, r.PathValue("slug"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string]Article{"article": result})
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	var input ArticleInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.Save(r.Context(), user, r.PathValue("id"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := 200
	if r.Method == http.MethodPost {
		status = 201
	}
	response.JSON(w, status, map[string]Article{"article": result})
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	var input DeleteInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.service.Delete(r.Context(), user, r.PathValue("id"), input.Version); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) view(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	var input struct{}
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.RecordView(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, result)
}
func (h *Handler) feedback(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	var input FeedbackInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.Feedback(r.Context(), user, r.PathValue("id"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, result)
}
func (h *Handler) categories(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	if err := allowedFilters(r, "q", "is_active", "limit", "offset"); err != nil {
		writeError(w, r, err)
		return
	}
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	f := CategoryFilter{Search: r.URL.Query().Get("q"), PublicOnly: !staff(user), Limit: limit, Offset: offset}
	if value := r.URL.Query().Get("is_active"); value != "" {
		if value != "true" && value != "false" {
			writeError(w, r, ErrInvalidInput)
			return
		}
		active := value == "true"
		f.Active = &active
	}
	result, err := h.service.Categories(r.Context(), user, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, result)
}
func (h *Handler) saveCategory(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	var input CategoryInput
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.SaveCategory(r.Context(), user, r.PathValue("id"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := 200
	if r.Method == http.MethodPost {
		status = 201
	}
	response.JSON(w, status, map[string]Category{"category": result})
}
func (h *Handler) tags(w http.ResponseWriter, r *http.Request) {
	user, ok := actor(w, r)
	if !ok {
		return
	}
	if err := allowedFilters(r, "q", "limit"); err != nil {
		writeError(w, r, err)
		return
	}
	limit, _, err := page(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.service.Tags(r.Context(), user, strings.TrimSpace(r.URL.Query().Get("q")), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 200, map[string][]string{"items": result})
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 500, "internal_error", "Unable to complete the knowledge operation"
	switch {
	case errors.Is(err, ErrInvalidInput):
		status, code, message = 400, "invalid_input", "Check the article fields, identifiers and filters"
	case errors.Is(err, ErrForbidden):
		status, code, message = 403, "forbidden", "This operation is not permitted"
	case errors.Is(err, ErrNotFound):
		status, code, message = 404, "not_found", "Knowledge resource not found"
	case errors.Is(err, ErrVersionConflict):
		status, code, message = 409, "version_conflict", "Article changed; reload before saving"
	case errors.Is(err, ErrFeedbackConflict):
		status, code, message = 409, "feedback_conflict", "Feedback for this attempt has already been recorded"
	case errors.Is(err, ErrInactiveCategory):
		status, code, message = 409, "inactive_category", "Choose an active category"
	case errors.Is(err, ErrArticleLinked):
		status, code, message = 409, "article_linked", "This article is referenced by a ticket; unpublish it instead"
	case errors.Is(err, ErrConflict):
		status, code, message = 409, "conflict", "A resource with this slug exists or the resource is in use"
	}
	response.Error(w, r, status, code, message)
}
