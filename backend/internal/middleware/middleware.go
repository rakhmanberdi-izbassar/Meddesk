// Package middleware contains cross-cutting HTTP controls.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/meddesk/meddesk/backend/internal/platform/requestmeta"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate our own ID: untrusted client headers must not enter logs or responses.
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(requestmeta.WithID(r.Context(), id)))
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// Deadline stops slow responses and propagates a deadline to database operations.
// Successful bounded downloads retain their MIME type; errors and timeouts
// keep the JSON envelope. Timeout buffering prevents partial attachments.
func Deadline(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(response.ErrorEnvelope{Error: response.ErrorDetail{
			Code: "request_timeout", Message: "Request timed out", RequestID: requestmeta.ID(r.Context()),
		}})
		http.TimeoutHandler(next, timeout, string(body)).ServeHTTP(jsonWriter{w}, r)
	})
}

// StreamDeadline leaves file responses unbuffered and bounds connection I/O.
// Once streaming starts, an expired deadline terminates I/O instead of replacing
// already-sent bytes with a JSON error. Database work receives the same deadline.
func StreamDeadline(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		deadline, _ := ctx.Deadline()
		controller := http.NewResponseController(w)
		// Test recorders do not support connection deadlines; real net/http writers do.
		_ = controller.SetReadDeadline(deadline)
		_ = controller.SetWriteDeadline(deadline)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type jsonWriter struct{ http.ResponseWriter }

func (w jsonWriter) WriteHeader(status int) {
	if status >= http.StatusBadRequest || w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.ResponseWriter.WriteHeader(status)
}

func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				logger.ErrorContext(r.Context(), "request handler panicked", "request_id", requestmeta.ID(r.Context()))
				response.Error(w, r, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// AccessLog obtains a normalized route from the router, never the raw URL or query.
func AccessLog(logger *slog.Logger, route func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorded := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(recorded, r)
		status := recorded.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.InfoContext(r.Context(), "http request", "method", r.Method, "route", route(r),
			"status", status, "duration_ms", time.Since(started).Milliseconds(), "request_id", requestmeta.ID(r.Context()))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
