package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/platform/requestmeta"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

func TestRequestIDIsGeneratedAndPropagated(t *testing.T) {
	var observed string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = requestmeta.ID(r.Context())
		w.WriteHeader(204)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", "untrusted-secret-value")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if observed == "" || observed == "untrusted-secret-value" || observed != w.Header().Get("X-Request-ID") {
		t.Fatal("request ID was not safely generated/propagated")
	}
}

func TestDeadlineCancelsContextAndReturnsCorrelatedJSON(t *testing.T) {
	canceled := make(chan struct{})
	h := RequestID(Deadline(20*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, exists := r.Context().Deadline(); !exists {
			t.Error("deadline is missing")
		}
		<-r.Context().Done()
		close(canceled)
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("handler was not canceled")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("incorrect content type: %s", w.Header().Get("Content-Type"))
	}
	var body response.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "request_timeout" || body.Error.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatalf("invalid timeout response: %+v", body)
	}
}

func TestDeadlinePreservesBoundedDownloadHeadersAndBytes(t *testing.T) {
	for _, contentType := range []string{"text/csv; charset=utf-8", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"} {
		t.Run(contentType, func(t *testing.T) {
			body := []byte{0xef, 0xbb, 0xbf, 't', 'i', 'c', 'k', 'e', 't'}
			h := Deadline(time.Second, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Content-Disposition", `attachment; filename="tickets.csv"`)
				w.Header().Set("Cache-Control", "private, no-store")
				w.Header().Set("Vary", "Cookie")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/tickets/export", nil))
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != contentType || !bytes.Equal(w.Body.Bytes(), body) {
				t.Fatal("deadline middleware changed the attachment MIME type or bytes")
			}
			if w.Header().Get("Content-Disposition") == "" || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Vary") != "Cookie" {
				t.Fatal("deadline middleware discarded private attachment headers")
			}
		})
	}
}

func TestDeadlineDiscardsAttachmentOnTimeout(t *testing.T) {
	done := make(chan struct{})
	h := RequestID(Deadline(20*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="tickets.csv"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("private partial data"))
		<-r.Context().Done()
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/tickets/export", nil))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("download handler was not canceled")
	}
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Content-Type") != "application/json; charset=utf-8" || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), "private partial data") {
		t.Fatal("timeout returned a partial attachment instead of the JSON error")
	}
	var body response.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != "request_timeout" {
		t.Fatalf("invalid attachment timeout envelope: %v", err)
	}
}

func TestRecoveryDoesNotExposePanicDetails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := RequestID(Recover(logger, Deadline(time.Second, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("secret-panic-body") }))))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "internal_error") {
		t.Fatalf("incorrect panic response: %s", w.Body.String())
	}
	if strings.Contains(logs.String()+w.Body.String(), "secret-panic-body") {
		t.Fatal("panic leaked sensitive information")
	}
}

func TestAccessLogRecordsStatusWithoutPrivateData(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := RequestID(AccessLog(logger, func(*http.Request) string { return "unmatched" }, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
		w.WriteHeader(500)
		_, _ = w.Write([]byte("private-response"))
	})))
	r := httptest.NewRequest("GET", "/secret-path?password=secret-query", strings.NewReader("secret-body"))
	r.Header.Set("Cookie", "secret-cookie")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 || !strings.Contains(logs.String(), `"status":404`) {
		t.Fatal("logging overwrote status")
	}
	for _, secret := range []string{"secret-path", "secret-query", "secret-body", "secret-cookie", "private-response"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("log leaked %s", secret)
		}
	}
}
