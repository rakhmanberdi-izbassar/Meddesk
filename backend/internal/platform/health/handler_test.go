package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type readinessStub struct{ err error }

func (s readinessStub) Ready(context.Context) error { return s.err }

func TestOperationalRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		wantStatus   int
		wantText     string
	}{
		{"GET", "/api/v1/health", 200, `"status":"ok"`},
		{"GET", "/api/v1/ready", 200, `"status":"ready"`},
		{"GET", "/api/v1/system", 200, `"phase":5`},
		{"GET", "/api/v1/openapi.json", 200, `"openapi": "3.1.0"`},
		{"GET", "/api/v1/missing?private=secret", 404, `"code":"not_found"`},
		{"POST", "/api/v1/ready", 405, `"code":"method_not_allowed"`},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			var logs bytes.Buffer
			h := NewHandler(readinessStub{}, "test-version").Router(slog.New(slog.NewJSONHandler(&logs, nil)), time.Second)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.wantStatus || !strings.Contains(w.Body.String(), tc.wantText) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatal("response must be JSON")
			}
			if w.Header().Get("X-Request-ID") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing security/correlation headers")
			}
			if strings.Contains(logs.String(), "private") || strings.Contains(logs.String(), "secret") || strings.Contains(logs.String(), "/missing") {
				t.Fatal("logs expose raw request URL")
			}
			if tc.wantStatus == 405 && w.Header().Get("Allow") != "GET, HEAD" {
				t.Fatal("missing Allow header")
			}
		})
	}
}

func TestLivenessIndependentAndReadinessPrivate(t *testing.T) {
	h := NewHandler(readinessStub{errors.New("database-password-secret")}, "0.1.0").Router(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), time.Second)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/health", nil))
	if w.Code != 200 {
		t.Fatal("liveness must not require database")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/ready", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("unexpected readiness response: %s", w.Body.String())
	}
	var envelope response.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatal("request ID does not match error")
	}
}
