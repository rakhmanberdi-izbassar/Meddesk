// Package health exposes operational status independently of feature authorization.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/meddesk/meddesk/backend/api/openapi"
	"github.com/meddesk/meddesk/backend/internal/middleware"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type Readiness interface{ Ready(context.Context) error }

type Handler struct {
	readiness Readiness
	version   string
}

func NewHandler(readiness Readiness, version string) *Handler {
	return &Handler{readiness: readiness, version: version}
}

func (h *Handler) Router(logger *slog.Logger, timeout time.Duration, register ...func(*http.ServeMux)) http.Handler {
	return h.RouterWithMedia(logger, timeout, 2*time.Minute, 5*time.Minute, register...)
}

func (h *Handler) RouterWithMedia(logger *slog.Logger, timeout, uploadTimeout, streamTimeout time.Duration, register ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", getOnly(h.live))
	mux.HandleFunc("/api/v1/ready", getOnly(h.ready))
	mux.HandleFunc("/api/v1/system", getOnly(h.system))
	mux.HandleFunc("/api/v1/openapi.json", getOnly(h.specification))
	for _, routes := range register {
		routes(mux)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, r, http.StatusNotFound, "not_found", "Resource not found")
	})
	route := func(r *http.Request) string {
		_, pattern := mux.Handler(r)
		if pattern == "/" || pattern == "" {
			return "unmatched"
		}
		return pattern
	}
	jsonHandler := middleware.Deadline(timeout, mux)
	uploadHandler := middleware.StreamDeadline(uploadTimeout, mux)
	streamHandler := middleware.StreamDeadline(streamTimeout, mux)
	deadlineHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		switch pattern {
		case "POST /api/v1/media":
			uploadHandler.ServeHTTP(w, r)
		case "GET /api/v1/media/{id}", "HEAD /api/v1/media/{id}":
			streamHandler.ServeHTTP(w, r)
		default:
			jsonHandler.ServeHTTP(w, r)
		}
	})
	return middleware.RequestID(middleware.SecurityHeaders(middleware.AccessLog(logger, route,
		middleware.Recover(logger, deadlineHandler))))
}

func getOnly(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			response.Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
			return
		}
		handler(w, r)
	}
}

func (h *Handler) live(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if err := h.readiness.Ready(r.Context()); err != nil {
		response.Error(w, r, http.StatusServiceUnavailable, "not_ready", "Service is not ready")
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
func (h *Handler) system(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, struct {
		Name    string `json:"name"`
		Phase   int    `json:"phase"`
		Version string `json:"version"`
	}{"MedDesk", 5, h.version})
}
func (h *Handler) specification(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapi.Document)
}
