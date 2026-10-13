package settings_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/audit"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/settings"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const origin = "https://meddesk.example.invalid"

type sessionRepo struct {
	session auth.Session
	hash    []byte
}

func (r *sessionRepo) FindUserByEmail(context.Context, string) (auth.Credential, error) {
	return auth.Credential{}, auth.ErrUnauthenticated
}
func (r *sessionRepo) FindSession(_ context.Context, h []byte) (auth.Session, error) {
	if !bytes.Equal(h, r.hash) {
		return auth.Session{}, auth.ErrUnauthenticated
	}
	return r.session, nil
}
func (r *sessionRepo) CreateSession(context.Context, auth.Credential, []byte, []byte, []byte, time.Time) (auth.User, error) {
	return auth.User{}, auth.ErrUnauthenticated
}
func (r *sessionRepo) RevokeSession(context.Context, []byte) error { return nil }

type settingsHTTP struct{ calls int }

func (s *settingsHTTP) Get(context.Context, auth.User) (settings.Settings, error) {
	s.calls++
	return settings.Settings{ClinicName: "Clinic", DefaultTicketPriority: "HIGH", ReportingTimezone: "Asia/Almaty", Version: 2}, nil
}
func (s *settingsHTTP) Update(_ context.Context, _ auth.User, in settings.UpdateInput) (settings.Settings, error) {
	s.calls++
	return settings.Settings{}, settings.ErrVersionConflict
}

type analyticsStore struct{ calls int }

func (s *analyticsStore) Report(context.Context, auth.User, analytics.Filter, time.Time) (analytics.Report, error) {
	s.calls++
	return analytics.Report{}, nil
}
func (s *analyticsStore) Filters(context.Context, auth.User) (analytics.Filters, error) {
	s.calls++
	return analytics.Filters{}, nil
}

type auditStore struct{ calls int }

func (s *auditStore) List(context.Context, auth.User, audit.Filter, time.Time) (audit.Page, error) {
	s.calls++
	return audit.Page{}, nil
}
func (s *auditStore) Filters(context.Context, auth.User) (audit.Filters, error) {
	s.calls++
	return audit.Filters{}, nil
}
func setup() (*http.ServeMux, *sessionRepo, *settingsHTTP, *analyticsStore, *auditStore, string, string) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	repo := &sessionRepo{auth.Session{User: auth.User{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Role: auth.RoleAdmin, IsActive: true}, CSRFHash: csrfHash[:], ExpiresAt: time.Now().Add(time.Hour)}, tokenHash[:]}
	guard := auth.NewGuard(auth.NewService(repo, time.Hour), auth.Options{AllowedOrigins: []string{origin}})
	mux := http.NewServeMux()
	sh, as, au := &settingsHTTP{}, &analyticsStore{}, &auditStore{}
	settings.NewHandler(sh, guard).RegisterRoutes(mux)
	analytics.NewHandler(analytics.NewService(as), guard).RegisterRoutes(mux)
	audit.NewHandler(audit.NewService(au), guard).RegisterRoutes(mux)
	return mux, repo, sh, as, au, token, csrf
}
func req(m *http.ServeMux, method, path, body, token, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", origin)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	if csrf != "" {
		r.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: csrf})
		r.Header.Set(auth.CSRFHeader, csrf)
	}
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	return w
}
func TestPhase5HTTPAuthenticationRolesAndCSRF(t *testing.T) {
	m, repo, _, _, _, token, csrf := setup()
	for _, path := range []string{"/api/v1/settings", "/api/v1/analytics", "/api/v1/analytics/filters", "/api/v1/admin/audit", "/api/v1/admin/audit/filters"} {
		if w := req(m, "GET", path, "", "", ""); w.Code != 401 {
			t.Fatal("anonymous endpoint", path, w.Code)
		}
	}
	if w := req(m, "PATCH", "/api/v1/settings", `{"version":1,"clinic_name":"Clinic"}`, token, ""); w.Code != 403 || !strings.Contains(w.Body.String(), "csrf_failed") {
		t.Fatal("missingcsrf", w.Code, w.Body.String())
	}
	repo.session.User.Role = auth.RoleEmployee
	for _, path := range []string{"/api/v1/analytics", "/api/v1/analytics/filters", "/api/v1/admin/audit", "/api/v1/admin/audit/filters"} {
		if w := req(m, "GET", path, "", token, csrf); w.Code != 403 {
			t.Fatal("employeeendpoint", path, w.Code)
		}
	}
	if w := req(m, "PATCH", "/api/v1/settings", `{"version":1}`, token, csrf); w.Code != 403 {
		t.Fatal("employeesettings", w.Code)
	}
	if w := req(m, "GET", "/api/v1/settings", "", token, csrf); w.Code != 200 || !strings.Contains(w.Body.String(), `"settings":{"clinic_name":"Clinic"`) {
		t.Fatal("publicsafe wrapper", w.Code, w.Body.String())
	}
	repo.session.User.Role = auth.RoleEngineer
	if w := req(m, "GET", "/api/v1/admin/audit", "", token, csrf); w.Code != 403 {
		t.Fatal("engineeraudit", w.Code)
	}
	if w := req(m, "GET", "/api/v1/analytics", "", token, csrf); w.Code != 200 {
		t.Fatal("engineeranalytics", w.Code)
	}
}
func TestPhase5HTTPRejectsFiltersAndJSONBeforeStore(t *testing.T) {
	m, _, sh, as, au, token, csrf := setup()
	cases := []string{"/api/v1/settings?unknown=1", "/api/v1/analytics?from=2026-01-01&from=2026-01-02", "/api/v1/analytics?from=", "/api/v1/analytics?unknown=1", "/api/v1/analytics?category_id=bad", "/api/v1/analytics?timezone=Local", "/api/v1/analytics?from=2024-01-01&to=2025-01-01", "/api/v1/analytics/filters?unknown=1", "/api/v1/admin/audit?limit=0", "/api/v1/admin/audit?limit=x", "/api/v1/admin/audit?offset=-1", "/api/v1/admin/audit?actor_id=bad", "/api/v1/admin/audit?entity_id=bad", "/api/v1/admin/audit?to=2026-02-30", "/api/v1/admin/audit?action=x&action=y", "/api/v1/admin/audit/filters?unknown=1"}
	for _, path := range cases {
		if w := req(m, "GET", path, "", token, csrf); w.Code != 400 {
			t.Fatal("badfilter accepted", path, w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"version":1,"password":"secret"}`, `{"version":1} {}`, `[]`} {
		if w := req(m, "PATCH", "/api/v1/settings", body, token, csrf); w.Code != 400 {
			t.Fatal("malformedjson", w.Code, w.Body.String())
		}
	}
	if w := req(m, "PATCH", "/api/v1/settings", `{"clinic_name":"`+strings.Repeat("a", 70000)+`"}`, token, csrf); w.Code != 413 {
		t.Fatal("unboundedbody", w.Code)
	}
	if sh.calls != 0 || as.calls != 0 || au.calls != 0 {
		t.Fatal("invalidrequests reached stores", sh.calls, as.calls, au.calls)
	}
	if w := req(m, "PATCH", "/api/v1/settings", `{"version":1,"clinic_name":"Clinic"}`, token, csrf); w.Code != 409 || !strings.Contains(w.Body.String(), "version_conflict") {
		t.Fatal("versionerror", w.Code, w.Body.String())
	}
}
