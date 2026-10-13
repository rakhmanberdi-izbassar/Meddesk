package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testOrigin = "https://meddesk.example.invalid"

func fixtureHandler(t *testing.T) (*http.ServeMux, *Handler, *memoryRepository) {
	t.Helper()
	s, repo := fixtureService(t)
	opts := Options{AllowedOrigins: []string{testOrigin}, CookieSecure: true}
	guard := NewGuard(s, opts)
	handler := NewHandler(s, guard, opts)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.Handle("POST /api/v1/admin-action", guard.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := UserFromContext(r.Context()); !ok || user.ID == "" {
			t.Error("missing user context")
		}
		w.WriteHeader(204)
	}), RoleAdmin))
	return mux, handler, repo
}
func loginHTTP(t *testing.T, mux *http.ServeMux) ([]*http.Cookie, string) {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"employee@example.invalid","password":"synthetic-password"}`))
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	var response authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return w.Result().Cookies(), response.CSRFToken
}
func authRequest(method, path string, cookies []*http.Cookie, csrf, origin string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set(CSRFHeader, csrf)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestHandlerLoginCookieMeAndLogout(t *testing.T) {
	mux, _, repo := fixtureHandler(t)
	cookies, csrf := loginHTTP(t, mux)
	if len(cookies) != 2 {
		t.Fatal("expected session and CSRF cookies")
	}
	for _, cookie := range cookies {
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != CookiePath || cookie.MaxAge < 1 || cookie.Expires.Before(time.Now()) {
			t.Fatalf("unsafe session cookie: %+v", cookie)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, authRequest("GET", "/api/v1/auth/me", cookies, "", ""))
	if w.Code != 200 || !strings.Contains(w.Body.String(), csrf) || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("me=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, authRequest("POST", "/api/v1/auth/logout", cookies, csrf, testOrigin))
	if w.Code != 204 || len(repo.sessions) != 0 {
		t.Fatalf("logout=%d", w.Code)
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.MaxAge != -1 || cookie.Path != CookiePath || !cookie.HttpOnly {
			t.Fatalf("cookie not expired safely: %+v", cookie)
		}
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, authRequest("GET", "/api/v1/auth/me", cookies, "", ""))
	if w.Code != 401 {
		t.Fatal("revoked session accepted")
	}
}

func TestGuardOriginCSRFAndRBAC(t *testing.T) {
	mux, _, repo := fixtureHandler(t)
	cookies, csrf := loginHTTP(t, mux)
	for _, tc := range []struct {
		name, origin, token string
		expected            int
		role                string
	}{
		{"employee denied", testOrigin, csrf, 403, RoleEmployee},
		{"admin allowed", testOrigin, csrf, 204, RoleAdmin},
		{"missing CSRF", testOrigin, "", 403, RoleAdmin},
		{"wrong CSRF", testOrigin, strings.Repeat("a", 43), 403, RoleAdmin},
		{"missing origin", "", csrf, 403, RoleAdmin},
		{"foreign origin", "https://attacker.invalid", csrf, 403, RoleAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo.credential.User.Role = tc.role
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, authRequest("POST", "/api/v1/admin-action", cookies, tc.token, tc.origin))
			if w.Code != tc.expected {
				t.Fatalf("expected%d got%d", tc.expected, w.Code)
			}
		})
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, authRequest("POST", "/api/v1/admin-action", nil, csrf, testOrigin))
	if w.Code != 401 {
		t.Fatal("unauthenticated mutation allowed")
	}
	repo.credential.User.IsActive = false
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, authRequest("GET", "/api/v1/auth/me", cookies, "", ""))
	if w.Code != 401 {
		t.Fatal("inactive account authenticated")
	}
}

func TestMeRequiresSessionBoundCSRFCookie(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("a", 43)} {
		t.Run(value, func(t *testing.T) {
			mux, _, _ := fixtureHandler(t)
			cookies, _ := loginHTTP(t, mux)
			for _, cookie := range cookies {
				if cookie.Name == CSRFCookie {
					cookie.Value = value
				}
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, authRequest("GET", "/api/v1/auth/me", cookies, "", ""))
			if w.Code != 401 {
				t.Fatalf("invalid CSRF cookie accepted: %d", w.Code)
			}
		})
	}
}

func TestLoginRejectsOriginAndInvalidBodies(t *testing.T) {
	for _, tc := range []struct {
		name, origin, body, contentType string
		status                          int
	}{
		{"missing origin", "", `{}`, "application/json", 403},
		{"foreign origin", "https://attacker.invalid", `{}`, "application/json", 403},
		{"unknown fields", testOrigin, `{"email":"employee@example.invalid","password":"synthetic-password","role":"ADMIN"}`, "application/json", 400},
		{"trailing JSON", testOrigin, `{} {}`, "application/json", 400},
		{"null", testOrigin, `null`, "application/json", 400},
		{"non UTF8", testOrigin, "{\"email\":\"\xff\"}", "application/json", 400},
		{"wrong content type", testOrigin, `{}`, "text/plain", 415},
		{"oversized", testOrigin, `{"email":"` + strings.Repeat("a", 4096) + `"}`, "application/json", 413},
		{"short password", testOrigin, `{"email":"employee@example.invalid","password":"short"}`, "application/json", 400},
		{"invalid credentials", testOrigin, `{"email":"employee@example.invalid","password":"incorrect-password"}`, "application/json", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, _, _ := fixtureHandler(t)
			r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("rejected login emitted cookies")
			}
		})
	}
}

func TestLoginRateLimitingByAccountAndPeer(t *testing.T) {
	mux, handler, _ := fixtureHandler(t)
	handler.accountLimiter = newLimiter(1, 10, time.Minute)
	loginHTTP(t, mux)
	r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":" Employee@Example.invalid ","password":"synthetic-password"}`))
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "192.0.2.2:1000"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("normalized account limit failed")
	}
	mux, handler, _ = fixtureHandler(t)
	handler.ipLimiter = newLimiter(1, 10, time.Minute)
	loginHTTP(t, mux)
	r = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"other@example.invalid","password":"synthetic-password"}`))
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Forwarded-For", "198.51.100.10")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatal("untrusted spoofed forwarded address bypassed peer limit")
	}
}
