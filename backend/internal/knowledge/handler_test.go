package knowledge

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

type sessionRepo struct {
	session auth.Session
	hash    []byte
}

func (r *sessionRepo) FindUserByEmail(context.Context, string) (auth.Credential, error) {
	return auth.Credential{}, auth.ErrUnauthenticated
}
func (r *sessionRepo) FindSession(_ context.Context, hash []byte) (auth.Session, error) {
	if !bytes.Equal(hash, r.hash) {
		return auth.Session{}, auth.ErrUnauthenticated
	}
	return r.session, nil
}
func (r *sessionRepo) CreateSession(context.Context, auth.Credential, []byte, []byte, []byte, time.Time) (auth.User, error) {
	return auth.User{}, auth.ErrUnauthenticated
}
func (r *sessionRepo) RevokeSession(context.Context, []byte) error { return nil }
func handlerFixture(t *testing.T) (*http.ServeMux, *sessionRepo, string, string) {
	t.Helper()
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	hash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	repo := &sessionRepo{session: auth.Session{User: engineer(), CSRFHash: csrfHash[:], ExpiresAt: time.Now().Add(time.Hour)}, hash: hash[:]}
	guard := auth.NewGuard(auth.NewService(repo, time.Hour), auth.Options{AllowedOrigins: []string{testOrigin}})
	mux := http.NewServeMux()
	service, _ := fixtureService()
	NewHandler(service, guard).RegisterRoutes(mux)
	return mux, repo, token, csrf
}
func request(mux *http.ServeMux, method, path, body, token, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Content-Type", "application/json")
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
func TestKnowledgeHandlerAuthenticationRoleAndCSRF(t *testing.T) {
	mux, repo, token, csrf := handlerFixture(t)
	cases := []struct {
		method, path, body, token, csrf string
		want                            int
	}{{"GET", "/api/v1/knowledge/articles", "", "", "", 401}, {"POST", "/api/v1/knowledge/articles", "{}", token, "", 403}, {"POST", "/api/v1/knowledge/categories", "{}", token, csrf, 403}}
	for _, tc := range cases {
		w := request(mux, tc.method, tc.path, tc.body, tc.token, tc.csrf)
		if w.Code != tc.want {
			t.Fatalf("%s %s got%d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	repo.session.User.Role = auth.RoleEmployee
	for _, path := range []string{"/api/v1/knowledge/articles?published=all", "/api/v1/knowledge/articles?published=false"} {
		w := request(mux, "GET", path, "", token, csrf)
		if w.Code != 403 {
			t.Fatalf("draft filter got%d: %s", w.Code, w.Body.String())
		}
	}
	w := request(mux, "DELETE", "/api/v1/knowledge/articles/"+articleID, `{"version":1}`, token, csrf)
	if w.Code != 403 {
		t.Fatalf("employee delete got%d", w.Code)
	}
}
func TestKnowledgeHandlerStrictFiltersAndJSON(t *testing.T) {
	mux, _, token, csrf := handlerFixture(t)
	cases := []struct {
		method, path, body string
		want               int
	}{{"GET", "/api/v1/knowledge/articles?limit=101", "", 400}, {"GET", "/api/v1/knowledge/articles?q=one&q=two", "", 400}, {"GET", "/api/v1/knowledge/articles?published=maybe", "", 400}, {"GET", "/api/v1/knowledge/articles?unknown=1", "", 400}, {"GET", "/api/v1/knowledge/articles?offset=", "", 400}, {"POST", "/api/v1/knowledge/articles", `{"title":"x","author_id":"fake"}`, 400}, {"POST", "/api/v1/knowledge/articles/" + articleID + "/views", `{"user_id":"fake"}`, 400}, {"POST", "/api/v1/knowledge/articles/" + articleID + "/feedback", `{"attempt_id":"bad","helpful":false}`, 400}, {"PATCH", "/api/v1/knowledge/articles/" + articleID, `{}`, 400}}
	for _, tc := range cases {
		w := request(mux, tc.method, tc.path, tc.body, token, csrf)
		if w.Code != tc.want {
			t.Fatalf("%s %s expected%d got%d: %s", tc.method, tc.path, tc.want, w.Code, w.Body.String())
		}
	}
	w := request(mux, "GET", "/api/v1/knowledge/articles?published=true&q=%D0%AD%D0%A6%D2%9A", "", token, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("valid search got%d:%s", w.Code, w.Body.String())
	}
}
