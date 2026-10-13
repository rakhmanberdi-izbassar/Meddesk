package tests

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/departments"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/health"
	"github.com/meddesk/meddesk/backend/internal/users"
)

// Exercise the composed router, real PostgreSQL repositories, and browser-style
// cookies together. Fixtures are synthetic and removed in the disposable DB.
func TestIdentityHTTPWorkflow(t *testing.T) {
	_, pool, authService, _, email, password := authFixture(t)
	// Race-instrumented bcrypt is substantially slower than production binaries;
	// bound this multi-login scenario independently of a single fixture setup.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	if _, err := pool.Exec(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='ADMIN') WHERE email=$1`, email); err != nil {
		t.Fatal(err)
	}
	options := auth.Options{AllowedOrigins: []string{"http://meddesk.test"}, SessionTTL: 8 * time.Hour}
	guard := auth.NewGuard(authService, options)
	authHandler := auth.NewHandler(authService, guard, options)
	userHandler := users.NewHandler(users.NewService(users.NewRepository(pool)), guard)
	departmentHandler := departments.NewHandler(departments.NewService(departments.NewRepository(pool)), guard)
	var logs bytes.Buffer
	router := health.NewHandler(health.NewService(pool, db.New(pool)), "test").Router(slog.New(slog.NewJSONHandler(&logs, nil)), 10*time.Second, authHandler.RegisterRoutes, userHandler.RegisterRoutes, departmentHandler.RegisterRoutes)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	newClient := func() *http.Client {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Client{Jar: jar, Timeout: 15 * time.Second}
	}
	admin := newClient()
	employee := newClient()
	call := func(client *http.Client, method, path, csrf string, body any, want int) (map[string]json.RawMessage, *http.Response) {
		t.Helper()
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		r, err := http.NewRequestWithContext(ctx, method, server.URL+"/api/v1"+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://meddesk.test")
		if csrf != "" {
			r.Header.Set(auth.CSRFHeader, csrf)
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s status=%d expected=%d", method, path, res.StatusCode, want)
		}
		if bytes.Contains(data, []byte("password_hash")) {
			t.Fatal("response leaked password hash")
		}
		var result map[string]json.RawMessage
		if len(data) > 0 {
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal("response is not JSON")
			}
		}
		return result, res
	}
	login := func(client *http.Client, address, secret string) (auth.User, string) {
		t.Helper()
		body, res := call(client, "POST", "/auth/login", "", map[string]string{"email": address, "password": secret}, 200)
		if len(res.Cookies()) != 2 {
			t.Fatal("session and CSRF cookies required")
		}
		for _, cookie := range res.Cookies() {
			if !cookie.HttpOnly || cookie.Path != "/api/v1" || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("unsafe cookie flags")
			}
		}
		var user auth.User
		var csrf string
		if err := json.Unmarshal(body["user"], &user); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body["csrf_token"], &csrf); err != nil || len(csrf) != 43 {
			t.Fatal("missing CSRF token")
		}
		return user, csrf
	}
	call(admin, "GET", "/users", "", nil, 401)
	adminUser, adminCSRF := login(admin, email, password)
	call(admin, "GET", "/auth/me", "", nil, 200)
	call(admin, "GET", "/users?limit=101", "", nil, 400)
	call(admin, "POST", "/departments", "", map[string]string{"name": "Must not be created"}, 403)
	departmentName := "HTTP fixture " + rand.Text()
	departmentResult, _ := call(admin, "POST", "/departments", adminCSRF, map[string]string{"name": departmentName}, 201)
	var department departments.Department
	if err := json.Unmarshal(departmentResult["department"], &department); err != nil {
		t.Fatal(err)
	}
	var employeeUser auth.User
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if employeeUser.ID != "" {
			if _, err := pool.Exec(clean, `DELETE FROM audit_logs WHERE actor_id=$1::uuid OR entity_id=$1::uuid`, employeeUser.ID); err != nil {
				t.Error(err)
			}
			if _, err := pool.Exec(clean, `DELETE FROM users WHERE id=$1::uuid`, employeeUser.ID); err != nil {
				t.Error(err)
			}
		}
		if _, err := pool.Exec(clean, `DELETE FROM audit_logs WHERE entity_id=$1::uuid`, department.ID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(clean, `DELETE FROM departments WHERE id=$1::uuid`, department.ID); err != nil {
			t.Error(err)
		}
	})
	employeeEmail := strings.ToLower(rand.Text()) + "@http.example.invalid"
	employeePassword := rand.Text()
	created, _ := call(admin, "POST", "/users", adminCSRF, map[string]any{"email": employeeEmail, "display_name": "Synthetic HTTP employee", "password": employeePassword, "role": auth.RoleEmployee, "department_id": department.ID}, 201)
	if err := json.Unmarshal(created["user"], &employeeUser); err != nil {
		t.Fatal(err)
	}
	_, employeeCSRF := login(employee, employeeEmail, employeePassword)
	call(employee, "GET", "/users", "", nil, 403)
	call(employee, "GET", "/departments", "", nil, 403)
	call(employee, "PATCH", "/profile", adminCSRF, map[string]string{"display_name": "Rejected"}, 403)
	call(employee, "PATCH", "/profile", employeeCSRF, map[string]string{"display_name": "Rejected", "role": auth.RoleAdmin}, 400)
	call(employee, "PATCH", "/profile", employeeCSRF, map[string]string{"display_name": "HTTP профиль"}, 200)
	call(admin, "PATCH", "/departments/"+department.ID, adminCSRF, map[string]any{"name": departmentName, "is_active": false}, 409)
	update := map[string]any{"email": employeeEmail, "display_name": "HTTP профиль", "role": auth.RoleEngineer, "department_id": department.ID, "is_active": true}
	call(admin, "PATCH", "/users/"+employeeUser.ID, adminCSRF, update, 200)
	call(employee, "GET", "/auth/me", "", nil, 401)
	engineer, _ := login(employee, employeeEmail, employeePassword)
	if engineer.Role != auth.RoleEngineer {
		t.Fatal("role not refreshed at login")
	}
	call(employee, "GET", "/users", "", nil, 403)
	newPassword := rand.Text()
	update["password"] = newPassword
	call(admin, "PATCH", "/users/"+employeeUser.ID, adminCSRF, update, 200)
	call(employee, "GET", "/auth/me", "", nil, 401)
	call(employee, "POST", "/auth/login", "", map[string]string{"email": employeeEmail, "password": employeePassword}, 401)
	login(employee, employeeEmail, newPassword)
	delete(update, "password")
	update["is_active"] = false
	call(admin, "PATCH", "/users/"+employeeUser.ID, adminCSRF, update, 200)
	call(employee, "GET", "/auth/me", "", nil, 401)
	call(employee, "POST", "/auth/login", "", map[string]string{"email": employeeEmail, "password": newPassword}, 401)
	call(admin, "PATCH", "/departments/"+department.ID, adminCSRF, map[string]any{"name": departmentName, "is_active": false}, 200)
	call(admin, "PATCH", "/users/"+adminUser.ID, adminCSRF, map[string]any{"email": adminUser.Email, "display_name": adminUser.DisplayName, "role": auth.RoleEmployee, "department_id": adminUser.DepartmentID, "is_active": true}, 403)
	call(admin, "POST", "/auth/logout", adminCSRF, nil, 204)
	call(admin, "GET", "/auth/me", "", nil, 401)
	if strings.Contains(logs.String(), employeePassword) || strings.Contains(logs.String(), newPassword) || strings.Contains(logs.String(), email) {
		t.Fatal("request secrets or identity leaked into access logs")
	}
}
