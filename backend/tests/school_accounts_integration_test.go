package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/school"
	"github.com/meddesk/meddesk/backend/internal/users"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Real sessions, real PostgreSQL, and the same guard used by every clinic API.
func TestSchoolAccountsHTTPIsolationAndRevocation(t *testing.T) {
	_, pool, authService, _, ownerEmail, password := authFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var ownerID string
	if err := pool.QueryRow(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='ADMIN') WHERE email=$1 RETURNING id::text`, ownerEmail).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO school_workspace(owner_id) VALUES($1)`, ownerID); err != nil {
		t.Fatal(err)
	}
	// This test only removes its synthetic accounts and school records.
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, sql := range []string{
			`DELETE FROM school_history WHERE actor_id=$1 OR actor_id IN(SELECT id FROM users WHERE email IN('a-'||$2,'b-'||$2))`,
			`DELETE FROM school_tickets WHERE requester_id IN(SELECT id FROM users WHERE email IN('a-'||$2,'b-'||$2)) AND $1::uuid IS NOT NULL`,
			`DELETE FROM school_account_events WHERE actor_id=$1 OR user_id IN(SELECT id FROM users WHERE email IN('a-'||$2,'b-'||$2))`,
			`DELETE FROM school_workspace WHERE owner_id=$1 AND $2::text IS NOT NULL`,
			`DELETE FROM users WHERE email IN('a-'||$2,'b-'||$2) AND $1::uuid IS NOT NULL`,
		} {
			if _, err := pool.Exec(clean, sql, ownerID, ownerEmail); err != nil {
				t.Error(err)
			}
		}
	})
	guard := auth.NewGuard(authService, auth.Options{AllowedOrigins: []string{"https://school.test"}})
	mux := http.NewServeMux()
	auth.NewHandler(authService, guard, auth.Options{AllowedOrigins: []string{"https://school.test"}}).RegisterRoutes(mux)
	school.NewHandler(school.NewService(school.NewRepository(pool)), guard).RegisterRoutes(mux)
	for _, path := range []string{"/api/v1/tickets", "/api/v1/knowledge/articles", "/api/v1/media/secret", "/api/v1/users", "/api/v1/settings", "/api/v1/analytics", "/api/v1/departments", "/api/v1/audit"} {
		mux.Handle("GET "+path, guard.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("school identity reached clinic handler")
			w.WriteHeader(200)
		})))
	}
	type client struct {
		cookies []*http.Cookie
		csrf    string
	}
	call := func(c *client, method, path string, body any, want int) []byte {
		t.Helper()
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(encoded)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://school.test")
		if c != nil {
			for _, cookie := range c.cookies {
				r.AddCookie(cookie)
			}
			r.Header.Set(auth.CSRFHeader, c.csrf)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: expected %d, got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	login := func(email, pass string) (client, auth.User) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "password": pass})
		r := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Origin", "https://school.test")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("login: %d", w.Code)
		}
		var result struct {
			User auth.User `json:"user"`
			CSRF string    `json:"csrf_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return client{cookies: w.Result().Cookies(), csrf: result.CSRF}, result.User
	}
	owner, ownerUser := login(ownerEmail, password)
	create := func(email string) string {
		t.Helper()
		body := call(&owner, "POST", "/api/v1/school/members", school.MemberInput{Email: email, DisplayName: "Мұғалім", Password: password}, 201)
		var m struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "password") {
			t.Fatal("password returned")
		}
		return m.ID
	}
	aID := create("a-" + ownerEmail)
	bID := create("b-" + ownerEmail)
	a, aUser := login("a-"+ownerEmail, password)
	b, _ := login("b-"+ownerEmail, password)
	if aUser.AccountScope != auth.AccountSchool || aUser.DepartmentID != "" {
		t.Fatalf("school login scope incorrect: %+v", aUser)
	}
	for _, path := range []string{"/api/v1/tickets", "/api/v1/knowledge/articles", "/api/v1/media/secret", "/api/v1/users", "/api/v1/settings", "/api/v1/analytics", "/api/v1/departments", "/api/v1/audit", "/api/v1/school/members", "/api/v1/school/assets", "/api/v1/school/tickets"} {
		call(&a, "GET", path, nil, 403)
	}
	call(&a, "POST", "/api/v1/school/members", school.MemberInput{Email: "forged@example.test", DisplayName: "Forged", Password: password}, 403)
	input := school.RequestInput{Title: "Wi-Fi істемейді", Description: "101 кабинетте желі жоқ", Room: "101", Category: "NETWORK", Priority: "MEDIUM"}
	data := call(&a, "POST", "/api/v1/school/requests", input, 201)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}
	forged := map[string]any{"title": input.Title, "description": input.Description, "room": "101", "category": "NETWORK", "priority": "MEDIUM", "requester_id": bID}
	call(&a, "POST", "/api/v1/school/requests", forged, 400)
	call(&b, "GET", "/api/v1/school/requests/"+created.ID, nil, 404)
	data = call(&b, "GET", "/api/v1/school/requests", nil, 200)
	if !bytes.Contains(data, []byte(`"total":0`)) {
		t.Fatalf("other employee saw request: %s", data)
	}
	call(&owner, "PATCH", "/api/v1/school/tickets/"+created.ID, school.UpdateInput{Status: "IN_PROGRESS", Priority: "HIGH", Version: 1}, 200)
	secret := "Private network diagnostics 192.0.2.42"
	call(&owner, "PATCH", "/api/v1/school/tickets/"+created.ID, school.UpdateInput{Status: "RESOLVED", Priority: "HIGH", Version: 2, Resolution: secret, PublicReply: "Желі қалпына келтірілді"}, 200)
	data = call(&a, "GET", "/api/v1/school/requests/"+created.ID, nil, 200)
	for _, private := range []string{secret, `"resolution"`, `"asset_id"`, `"requester_id"`} {
		if bytes.Contains(data, []byte(private)) {
			t.Fatalf("private field leaked: %s", data)
		}
	}
	if !bytes.Contains(data, []byte("Желі қалпына келтірілді")) {
		t.Fatal("public reply missing")
	}
	// Clinic account management cannot enumerate or reset school accounts.
	page, err := users.NewService(users.NewRepository(pool)).List(ctx, ownerUser, users.Filter{Search: "a-" + ownerEmail, Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("school account leaked into clinic management: %+v %v", page, err)
	}
	if _, err = users.NewService(users.NewRepository(pool)).Update(ctx, ownerUser, aID, users.UpdateInput{Email: "a-" + ownerEmail, DisplayName: "Changed", Role: auth.RoleAdmin, DepartmentID: ownerUser.DepartmentID, IsActive: boolValue(true)}); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("clinic updated school identity: %v", err)
	}
	var auditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id IN($1,$2)`, aID, bID).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("school audit leaked into clinic: %d %v", auditCount, err)
	}
	newPassword := "new-synthetic-password-2026"
	call(&owner, "PATCH", "/api/v1/school/members/"+aID, school.MemberUpdate{DisplayName: "Мұғалім A", IsActive: boolValue(true), Version: 1, Password: &newPassword}, 200)
	call(&a, "GET", "/api/v1/school/requests", nil, 401)
	a, _ = login("a-"+ownerEmail, newPassword)
	call(&a, "GET", "/api/v1/school/requests", nil, 200)
	call(&owner, "PATCH", "/api/v1/school/members/"+bID, school.MemberUpdate{DisplayName: "Мұғалім B", IsActive: boolValue(false), Version: 1}, 200)
	call(&b, "GET", "/api/v1/school/requests", nil, 401)
	call(&owner, "PATCH", "/api/v1/school/members/"+aID, school.MemberUpdate{DisplayName: "Stale", IsActive: boolValue(false), Version: 1}, 409)
	// Scope remains authoritative even if a privileged database operator assigns a
	// clinic role by mistake: no clinic privilege or administrator-count inflation.
	q := db.New(pool)
	before, err := q.CountActiveAdministrators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='ADMIN') WHERE id=$1`, aID); err != nil {
		t.Fatal(err)
	}
	call(&a, "GET", "/api/v1/settings", nil, 403)
	after, err := q.CountActiveAdministrators(ctx)
	if err != nil || after != before {
		t.Fatalf("school role inflated clinic admin count: %d %d %v", before, after, err)
	}
	choices, err := q.AnalyticsFilterAssignees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range choices {
		data, _ := json.Marshal(choice.ID)
		if string(data) == `"`+aID+`"` {
			t.Fatal("school identity leaked through clinic engineer filters")
		}
	}

}
