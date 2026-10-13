package tickets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
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
func handlerFixture() (*http.ServeMux, *sessionRepo, *fakeStore, string, string) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	repo := &sessionRepo{session: auth.Session{User: engineer(), CSRFHash: csrfHash[:], ExpiresAt: time.Now().Add(time.Hour)}, hash: tokenHash[:]}
	guard := auth.NewGuard(auth.NewService(repo, time.Hour), auth.Options{AllowedOrigins: []string{testOrigin}})
	s, store := fixture()
	mux := http.NewServeMux()
	NewHandler(s, guard).RegisterRoutes(mux)
	return mux, repo, store, token, csrf
}
func request(mux *http.ServeMux, method, path, body, token, csrf string) *httptest.ResponseRecorder {
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
func TestTicketHandlerAuthCSRFAndRoles(t *testing.T) {
	mux, repo, _, token, csrf := handlerFixture()
	cases := []struct {
		method, path, body, token, csrf string
		want                            int
	}{{"GET", "/api/v1/tickets", "", "", "", 401}, {"POST", "/api/v1/tickets", "{}", token, "", 403}, {"POST", "/api/v1/tickets/" + ticketID + "/reopen", `{"version":1,"reason":"Retry"}`, token, csrf, 403}}
	for _, tc := range cases {
		w := request(mux, tc.method, tc.path, tc.body, tc.token, tc.csrf)
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	repo.session.User = employee()
	for _, path := range []string{"/api/v1/tickets/assignees", "/api/v1/tickets/departments"} {
		w := request(mux, "GET", path, "", token, csrf)
		if w.Code != 403 {
			t.Fatalf("employee catalog accepted: %d", w.Code)
		}
	}
	w := request(mux, "PATCH", "/api/v1/tickets/"+ticketID, `{"version":1,"priority":"HIGH"}`, token, csrf)
	if w.Code != 403 {
		t.Fatalf("employee patch accepted: %d", w.Code)
	}
}
func TestTicketHandlerRejectsUnsupportedRepeatedAndMalformedFilters(t *testing.T) {
	mux, _, _, token, csrf := handlerFixture()
	for _, query := range []string{"unknown=1", "status=NEW&status=CLOSED", "limit=no", "offset=-1", "mine=1", "priority=URGENT", "department_id=invalid", "q="} {
		w := request(mux, "GET", "/api/v1/tickets?"+query, "", token, csrf)
		if w.Code != 400 {
			t.Errorf("%s: got %d %s", query, w.Code, w.Body.String())
		}
	}
}
func TestTicketHandlerBoundedJSONAndVersionErrors(t *testing.T) {
	mux, _, store, token, csrf := handlerFixture()
	for _, body := range []string{`{"title":"Support","description":"Problem","requester_id":"` + actorID + `"}`, `{"title":"Support","description":"Problem"} {}`, `[]`, `{"title":"` + strings.Repeat("x", 70000) + `"}`} {
		w := request(mux, "POST", "/api/v1/tickets", body, token, csrf)
		want := 400
		if len(body) > 65536 {
			want = 413
		}
		if w.Code != want {
			t.Fatalf("malformed body accepted: got%d", w.Code)
		}
	}
	if store.transactions != 0 {
		t.Fatal("invalid JSON reached transaction")
	}
	w := request(mux, "PATCH", "/api/v1/tickets/"+ticketID, `{"version":2,"priority":"HIGH"}`, token, csrf)
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"version_conflict"`) {
		t.Fatalf("version error contract wrong: %d %s", w.Code, w.Body.String())
	}
}
func TestTicketHandlerExplicitNullAssignment(t *testing.T) {
	mux, _, store, token, csrf := handlerFixture()
	store.tx.ticket.AssigneeID = str(engineerID)
	w := request(mux, "PATCH", "/api/v1/tickets/"+ticketID, `{"version":1,"assignee_id":null}`, token, csrf)
	if w.Code != 200 || store.tx.ticket.AssigneeID != nil || !strings.Contains(w.Body.String(), `"ticket":`) {
		t.Fatalf("explicit null not applied: %d %s", w.Code, w.Body.String())
	}
}
func TestTicketExportHandlerAuthFiltersAndAttachment(t *testing.T) {
	mux, repo, store, token, _ := handlerFixture()
	if w := request(mux, "GET", "/api/v1/tickets/export", "", "", ""); w.Code != 401 {
		t.Fatalf("unauthenticated export: %d", w.Code)
	}
	repo.session.User = employee()
	if w := request(mux, "GET", "/api/v1/tickets/export", "", token, ""); w.Code != 403 {
		t.Fatalf("employee export: %d", w.Code)
	}
	repo.session.User = engineer()
	for _, query := range []string{"limit=20", "offset=0", "format=pdf", "locale=en", "mine=1", "from=2026-10-12", "to=2026-10-12", "timezone=Local", "format=csv&format=xlsx", "locale=", "status=%GG", "q=ignored;status=NEW"} {
		w := request(mux, "GET", "/api/v1/tickets/export?"+query, "", token, "")
		if w.Code != 400 || w.Header().Get("Content-Disposition") != "" {
			t.Errorf("invalid export %q: %d %s", query, w.Code, w.Body.String())
		}
	}
	w := request(mux, "GET", "/api/v1/tickets/export?format=csv&locale=ru&mine=true&from=2026-10-12&to=2026-10-12&timezone=UTC", "", token, "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), `attachment; filename="meddesk-tickets-`) || !strings.HasSuffix(w.Header().Get("Content-Disposition"), `.csv"`) || w.Header().Get("Content-Type") != "text/csv; charset=utf-8" || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Vary") != "Cookie" {
		t.Fatalf("invalid export attachment: %d %+v", w.Code, w.Header())
	}
	if !store.exportFilter.Mine || store.exportFilter.Timezone != "UTC" || store.exportFilter.From != "2026-10-12" || store.exportFilter.To != "2026-10-12" {
		t.Fatal("handler did not preserve selected filters")
	}
	store.exportErr = ErrExportLimit
	w = request(mux, "GET", "/api/v1/tickets/export", "", token, "")
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"export_limit"`) || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("limit must produce JSON before attachment: %d %s", w.Code, w.Body.String())
	}
}
