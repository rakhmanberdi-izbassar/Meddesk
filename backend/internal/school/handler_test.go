package school

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type sessionRepository struct {
	token   []byte
	session auth.Session
}

func (r *sessionRepository) FindUserByEmail(context.Context, string) (auth.Credential, error) {
	return auth.Credential{}, auth.ErrUnauthenticated
}
func (r *sessionRepository) FindSession(_ context.Context, token []byte) (auth.Session, error) {
	if !bytes.Equal(token, r.token) {
		return auth.Session{}, auth.ErrUnauthenticated
	}
	return r.session, nil
}
func (r *sessionRepository) CreateSession(context.Context, auth.Credential, []byte, []byte, []byte, time.Time) (auth.User, error) {
	return auth.User{}, auth.ErrUnauthenticated
}
func (r *sessionRepository) RevokeSession(context.Context, []byte) error { return nil }
func TestSchoolHandlersRequireSessionAndCSRF(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	repo := &sessionRepository{token: tokenHash[:], session: auth.Session{User: auth.User{ID: "00000000-0000-4000-8000-000000000001", IsActive: true, Role: auth.RoleAdmin}, CSRFHash: csrfHash[:], ExpiresAt: time.Now().Add(time.Hour)}}
	guard := auth.NewGuard(auth.NewService(repo, time.Hour), auth.Options{AllowedOrigins: []string{"https://school.example.test"}})
	mux := http.NewServeMux()
	NewHandler(nil, guard).RegisterRoutes(mux)
	for _, tc := range []struct {
		method, path                    string
		session, validOrigin, validCSRF bool
		status                          int
	}{
		{"GET", "/api/v1/school/assets", false, false, false, 401},
		{"GET", "/api/v1/school/tickets", false, false, false, 401},
		{"POST", "/api/v1/school/tickets", true, false, true, 403},
		{"POST", "/api/v1/school/assets", true, true, false, 403},
		{"PATCH", "/api/v1/school/tickets/00000000-0000-4000-8000-000000000002", true, true, false, 403},
		{"GET", "/api/v1/school/tickets?limit=not-a-number", true, false, false, 400},
		{"GET", "/api/v1/school/assets?status=NEW", true, false, false, 400},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			if tc.session {
				r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
			}
			if tc.validOrigin {
				r.Header.Set("Origin", "https://school.example.test")
			}
			if tc.validCSRF {
				r.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: csrf})
				r.Header.Set(auth.CSRFHeader, csrf)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
