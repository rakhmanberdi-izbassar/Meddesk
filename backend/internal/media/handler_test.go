package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
)

const testOrigin = "https://meddesk.example.invalid"

var testToken = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
var testCSRF = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))

type guardRepository struct{ user auth.User }

func (r *guardRepository) FindUserByEmail(context.Context, string) (auth.Credential, error) {
	return auth.Credential{}, errors.New("not used")
}
func (r *guardRepository) FindSession(_ context.Context, hash []byte) (auth.Session, error) {
	expected := sha256.Sum256([]byte(testToken))
	if !bytes.Equal(hash, expected[:]) {
		return auth.Session{}, auth.ErrUnauthenticated
	}
	csrf := sha256.Sum256([]byte(testCSRF))
	return auth.Session{User: r.user, CSRFHash: csrf[:], ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (r *guardRepository) CreateSession(context.Context, auth.Credential, []byte, []byte, []byte, time.Time) (auth.User, error) {
	return auth.User{}, errors.New("not used")
}
func (r *guardRepository) RevokeSession(context.Context, []byte) error { return errors.New("not used") }
func handlerFixture(t *testing.T) (*http.ServeMux, *Service, *memoryStore, *guardRepository) {
	t.Helper()
	service, store, _ := fixtureService(t, Options{})
	repo := &guardRepository{user: staffUser()}
	guard := auth.NewGuard(auth.NewService(repo, time.Hour), auth.Options{AllowedOrigins: []string{testOrigin}})
	mux := http.NewServeMux()
	NewHandler(service, guard).RegisterRoutes(mux)
	return mux, service, store, repo
}
func requestWithSession(method, path string, body []byte) *http.Request {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: testToken})
	r.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: testCSRF})
	r.Header.Set("Origin", testOrigin)
	r.Header.Set(auth.CSRFHeader, testCSRF)
	return r
}
func multipartBody(t *testing.T, parts int, name string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for i := 0; i < parts; i++ {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+name+`"; filename="screen.png"`)
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(testPNG(t)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}
func TestUploadHandlerBoundedMultipartAndAuthorization(t *testing.T) {
	mux, _, store, repo := handlerFixture(t)
	for _, tc := range []struct {
		name, path, role, csrf, partName string
		parts, status                    int
		truncate                         bool
	}{
		{"upload", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEngineer, testCSRF, "file", 1, 201, false},
		{"admin upload", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleAdmin, testCSRF, "file", 1, 201, false},
		{"employee", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEmployee, testCSRF, "file", 1, 403, false},
		{"employee screenshot", "/api/v1/media?purpose=TICKET_ATTACHMENT", auth.RoleEmployee, testCSRF, "file", 1, 201, false},
		{"employee screenshot requires CSRF", "/api/v1/media?purpose=TICKET_ATTACHMENT", auth.RoleEmployee, "", "file", 1, 403, false},
		{"missing CSRF", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEngineer, "", "file", 1, 403, false},
		{"duplicate file", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEngineer, testCSRF, "file", 2, 400, false},
		{"wrong field", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEngineer, testCSRF, "other", 1, 400, false},
		{"no file", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEngineer, testCSRF, "file", 0, 400, false},
		{"missing purpose", "/api/v1/media", auth.RoleEngineer, testCSRF, "file", 1, 400, false},
		{"unknown query", "/api/v1/media?purpose=ARTICLE_IMAGE&filename=hack", auth.RoleEngineer, testCSRF, "file", 1, 400, false},
		{"repeat purpose", "/api/v1/media?purpose=ARTICLE_IMAGE&purpose=ARTICLE_VIDEO", auth.RoleEngineer, testCSRF, "file", 1, 400, false},
		{"truncated closing boundary", "/api/v1/media?purpose=ARTICLE_IMAGE", auth.RoleEngineer, testCSRF, "file", 1, 400, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.created = 0
			repo.user.Role = tc.role
			body, contentType := multipartBody(t, tc.parts, tc.partName)
			if tc.truncate {
				body = body[:len(body)-20]
			}
			r := requestWithSession("POST", tc.path, body)
			r.Header.Set("Content-Type", contentType)
			r.Header.Set(auth.CSRFHeader, tc.csrf)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("expected %d got %d %s", tc.status, w.Code, w.Body.String())
			}
			if tc.status == 201 {
				var result struct {
					Media File `json:"media"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Media.ID == "" || result.Media.URL == "" || strings.Contains(w.Body.String(), "storage_key") || strings.Contains(w.Body.String(), "sha256") {
					t.Fatal("unsafe upload DTO")
				}
			} else if store.created != 0 {
				t.Fatal("rejected upload committed metadata")
			}
		})
	}
	body, contentType := multipartBody(t, 1, "file")
	r := httptest.NewRequest("POST", "/api/v1/media?purpose=ARTICLE_IMAGE", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous upload allowed")
	}
}
func TestDownloadHandlerRangesHeadAndCacheAuthorization(t *testing.T) {
	mux, service, _, _ := handlerFixture(t)
	data := testPNG(t)
	file, err := service.Upload(context.Background(), staffUser(), UploadInput{Purpose: PurposeImage, Filename: "screen.png", Reader: bytes.NewReader(data)})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, rang string
		status       int
		length       int
	}{{"GET", "", 200, len(data)}, {"HEAD", "", 200, 0}, {"GET", "bytes=0-9", 206, 10}, {"GET", "bytes=99999-", 416, 0}} {
		r := requestWithSession(tc.method, file.URL, nil)
		r.Header.Set("Range", tc.rang)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %q got %d", tc.method, tc.rang, w.Code)
		}
		if tc.status != 416 && w.Body.Len() != tc.length {
			t.Fatalf("incorrect response length: %d", w.Body.Len())
		}
		if tc.status == 206 && w.Header().Get("Content-Range") == "" {
			t.Fatal("missing range header")
		}
		if tc.status != 416 && (w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Type") != "image/png") {
			t.Fatal("unsafe media response headers")
		}
	}
	r := httptest.NewRequest("GET", file.URL, nil)
	r.Header.Set("If-None-Match", `"`+file.ID+`"`)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous conditional request bypassed authentication")
	}
	r = requestWithSession("GET", file.URL, nil)
	r.Header.Set("If-None-Match", `"`+file.ID+`"`)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal("authorized conditional response failed")
	}
}
