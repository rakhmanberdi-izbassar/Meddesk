package users

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

const testOrigin = "https://meddesk.example.invalid"

type sessionRepository struct {
	session auth.Session
	hash    []byte
}

func (r *sessionRepository) FindUserByEmail(context.Context, string) (auth.Credential, error) {
	return auth.Credential{}, auth.ErrUnauthenticated
}
func (r *sessionRepository) FindSession(_ context.Context, hash []byte) (auth.Session, error) {
	if !bytes.Equal(hash, r.hash) {
		return auth.Session{}, auth.ErrUnauthenticated
	}
	return r.session, nil
}
func (r *sessionRepository) CreateSession(context.Context, auth.Credential, []byte, []byte, []byte, time.Time) (auth.User, error) {
	return auth.User{}, auth.ErrUnauthenticated
}
func (r *sessionRepository) RevokeSession(context.Context, []byte) error { return nil }
func handlerFixture(t *testing.T) (*http.ServeMux, *sessionRepository, string, string) {
	t.Helper()
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	// Hash auth's canonical textual token, matching the repository contract.
	repo := &sessionRepository{session: auth.Session{User: adminActor(), CSRFHash: csrfHash[:], ExpiresAt: time.Now().Add(time.Hour)}, hash: tokenHash[:]}
	guard := auth.NewGuard(auth.NewService(repo, time.Hour), auth.Options{AllowedOrigins: []string{testOrigin}})
	mux := http.NewServeMux()
	NewHandler(NewService(newFixture()), guard).RegisterRoutes(mux)
	return mux, repo, token, csrf
}
func handlerRequest(mux *http.ServeMux, method, path, body, token, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", testOrigin)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	if csrf != "" {
		r.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: csrf})
		r.Header.Set(auth.CSRFHeader, csrf)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func TestManagementHandlerRequiresAuthenticationRBACAndCSRF(t *testing.T) {
	mux, repo, token, csrf := handlerFixture(t)
	w := handlerRequest(mux, "GET", "/api/v1/users", "", "", "")
	if w.Code != 401 {
		t.Fatalf("unauthenticated request: %d", w.Code)
	}
	repo.session.User.Role = auth.RoleEmployee
	w = handlerRequest(mux, "GET", "/api/v1/users", "", token, csrf)
	if w.Code != 403 {
		t.Fatalf("employee admin request: %d", w.Code)
	}
	repo.session.User = adminActor()
	w = handlerRequest(mux, "POST", "/api/v1/users", `{}`, token, "")
	if w.Code != 403 {
		t.Fatalf("unprotected mutation: %d", w.Code)
	}
}
func TestManagementHandlerStrictParsingAndProfile(t *testing.T) {
	mux, repo, token, csrf := handlerFixture(t)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/v1/users?limit=101", "", 400},
		{"GET", "/api/v1/users?role=SUPERUSER", "", 400},
		{"GET", "/api/v1/users?q=one&q=two", "", 400},
		{"PATCH", "/api/v1/profile", `{"display_name":"Updated","role":"ADMIN"}`, 400},
		{"PATCH", "/api/v1/profile", `{} {}`, 400},
		{"PATCH", "/api/v1/profile", `{"display_name":"` + strings.Repeat("a", 70000) + `"}`, 413},
		{"PATCH", "/api/v1/users/" + adminID, `{"email":"admin@example.invalid","display_name":"Admin","role":"EMPLOYEE","department_id":"` + departmentID + `","is_active":true}`, 403},
	} {
		w := handlerRequest(mux, tc.method, tc.path, tc.body, token, csrf)
		if w.Code != tc.status {
			t.Fatalf("%s %s expected%d got%d: %s", tc.method, tc.path, tc.status, w.Code, w.Body.String())
		}
	}
	repo.session.User = auth.User{ID: employeeID, Role: auth.RoleEmployee, IsActive: true}
	w := handlerRequest(mux, "PATCH", "/api/v1/profile", `{"display_name":"Жаңа атау"}`, token, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Жаңа атау") || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("profile=%d %s", w.Code, w.Body.String())
	}
}
