package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/media"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/health"
)

// Cover authorization across the article publication boundary using real HTTP,
// browser cookies, PostgreSQL transactions, and private filesystem storage.
func TestKnowledgeMediaHTTPPublicationWorkflow(t *testing.T) {
	_, pool, authentication, _, email, password := authFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	var adminID, employeeID, employeeEmail, categoryID, articleID, mediaID string
	if err := pool.QueryRow(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='ADMIN') WHERE email=$1 RETURNING id::text`, email).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,display_name,password_hash,role_id,department_id) SELECT gen_random_uuid()::text || '@example.invalid','Synthetic HTTP knowledge employee',password_hash,(SELECT id FROM roles WHERE code='EMPLOYEE'),department_id FROM users WHERE id=$1 RETURNING id::text,email`, adminID).Scan(&employeeID, &employeeEmail); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, operation := range []struct{ sql, id string }{
			{`DELETE FROM knowledge_articles WHERE id=$1::uuid`, articleID},
			{`DELETE FROM media_files WHERE id=$1::uuid`, mediaID},
			{`DELETE FROM knowledge_categories WHERE id=$1::uuid`, categoryID},
			{`DELETE FROM audit_logs WHERE actor_id=$1::uuid OR entity_id=$1::uuid`, employeeID},
			{`DELETE FROM users WHERE id=$1::uuid`, employeeID},
		} {
			if operation.id != "" {
				if _, err := pool.Exec(clean, operation.sql, operation.id); err != nil {
					t.Error("unable to remove synthetic knowledge fixture")
				}
			}
		}
		for _, id := range []string{articleID, mediaID, categoryID} {
			if id != "" {
				if _, err := pool.Exec(clean, `DELETE FROM audit_logs WHERE entity_id=$1::uuid`, id); err != nil {
					t.Error("unable to remove synthetic knowledge audit")
				}
			}
		}
	})
	files, err := media.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	options := auth.Options{AllowedOrigins: []string{"http://meddesk.test"}, SessionTTL: 8 * time.Hour}
	guard := auth.NewGuard(authentication, options)
	authHandler := auth.NewHandler(authentication, guard, options)
	knowledgeHandler := knowledge.NewHandler(knowledge.NewService(knowledge.NewRepository(pool)), guard)
	mediaHandler := media.NewHandler(media.NewService(media.NewRepository(pool), files, media.Options{}), guard)
	router := health.NewHandler(health.NewService(pool, db.New(pool)), "test").RouterWithMedia(slog.New(slog.NewJSONHandler(io.Discard, nil)), 10*time.Second, 2*time.Minute, 5*time.Minute, authHandler.RegisterRoutes, knowledgeHandler.RegisterRoutes, mediaHandler.RegisterRoutes)
	server := httptest.NewServer(router)
	defer server.Close()
	type browser struct {
		client *http.Client
		csrf   string
	}
	newBrowser := func() *browser {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &browser{client: &http.Client{Jar: jar, Timeout: 15 * time.Second}}
	}
	admin, employee, anonymous := newBrowser(), newBrowser(), newBrowser()
	send := func(actor *browser, method, path string, payload []byte, contentType string, headers map[string]string, want int) ([]byte, *http.Response) {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, method, server.URL+"/api/v1"+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal("unable to build integration request")
		}
		request.Header.Set("Origin", "http://meddesk.test")
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		if actor.csrf != "" {
			request.Header.Set(auth.CSRFHeader, actor.csrf)
		}
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		result, err := actor.client.Do(request)
		if err != nil {
			t.Fatalf("%s %s request failed", method, path)
		}
		data, err := io.ReadAll(result.Body)
		_ = result.Body.Close()
		if err != nil {
			t.Fatalf("%s %s response read failed", method, path)
		}
		if result.StatusCode != want {
			t.Fatalf("%s %s status=%d expected=%d", method, path, result.StatusCode, want)
		}
		return data, result
	}
	call := func(actor *browser, method, path string, body any, want int) []byte {
		t.Helper()
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatal("unable to encode integration input")
			}
		}
		data, _ := send(actor, method, path, payload, "application/json", nil, want)
		return data
	}
	decode := func(data []byte, destination any) {
		t.Helper()
		if err := json.Unmarshal(data, destination); err != nil {
			t.Fatal("unexpected integration response shape")
		}
	}
	login := func(actor *browser, address string) {
		t.Helper()
		data := call(actor, "POST", "/auth/login", map[string]string{"email": address, "password": password}, 200)
		var session struct {
			User auth.User `json:"user"`
			CSRF string    `json:"csrf_token"`
		}
		decode(data, &session)
		if !session.User.IsActive || len(session.CSRF) != 43 {
			t.Fatal("invalid browser session")
		}
		actor.csrf = session.CSRF
	}
	call(anonymous, "GET", "/knowledge/articles", nil, 401)
	login(admin, email)
	login(employee, employeeEmail)
	categoryInput := map[string]any{"slug": "http-" + adminID, "name_kk": "HTTP демо", "name_ru": "HTTP демо", "description": "Synthetic test category", "is_active": true}
	call(employee, "POST", "/knowledge/categories", categoryInput, 403)
	var categoryResult struct {
		Category knowledge.Category `json:"category"`
	}
	decode(call(admin, "POST", "/knowledge/categories", categoryInput, 201), &categoryResult)
	categoryID = categoryResult.Category.ID
	if categoryID == "" {
		t.Fatal("category identifier missing")
	}
	picture := mediaPNG(t)
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="synthetic-screen.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(picture); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadPath := "/media?purpose=ARTICLE_IMAGE"
	send(admin, "POST", uploadPath, multipartBody.Bytes(), writer.FormDataContentType(), map[string]string{auth.CSRFHeader: ""}, 403)
	send(employee, "POST", uploadPath, multipartBody.Bytes(), writer.FormDataContentType(), nil, 403)
	uploaded, _ := send(admin, "POST", uploadPath, multipartBody.Bytes(), writer.FormDataContentType(), nil, 201)
	if bytes.Contains(uploaded, []byte("storage_key")) || bytes.Contains(uploaded, []byte("sha256")) {
		t.Fatal("upload exposed private storage metadata")
	}
	var mediaResult struct {
		Media media.File `json:"media"`
	}
	decode(uploaded, &mediaResult)
	mediaID = mediaResult.Media.ID
	if mediaID == "" || mediaResult.Media.URL != "/api/v1/media/"+mediaID || mediaResult.Media.ByteCount != int64(len(picture)) {
		t.Fatal("incorrect uploaded media metadata")
	}
	mediaPath := "/media/" + mediaID
	call(employee, "GET", mediaPath, nil, 404)
	input := map[string]any{"title": "NCALayer HTTP нұсқаулық", "slug": "http-" + adminID, "description": "Synthetic self-service guide", "category_id": categoryID, "symptoms": "ЭЦҚ кірмейді", "solution_steps": []string{"NCALayer іске қосыңыз", "Қайта тексеріңіз"}, "tags": []string{"http-demo"}, "video_id": nil, "image_ids": []string{mediaID}, "is_published": false}
	var articleResult struct {
		Article knowledge.Article `json:"article"`
	}
	decode(call(admin, "POST", "/knowledge/articles", input, 201), &articleResult)
	articleID = articleResult.Article.ID
	articlePath := "/knowledge/articles/" + articleID
	if articleID == "" || articleResult.Article.Version != 1 || len(articleResult.Article.ImageIDs) != 1 {
		t.Fatal("invalid draft article")
	}
	call(employee, "GET", articlePath, nil, 404)
	call(employee, "GET", mediaPath, nil, 404)
	input["version"] = articleResult.Article.Version
	input["is_published"] = true
	decode(call(admin, "PATCH", articlePath, input, 200), &articleResult)
	if articleResult.Article.Version != 2 || !articleResult.Article.IsPublished {
		t.Fatal("publication/version did not persist")
	}
	call(employee, "GET", articlePath, nil, 200)
	data, response := send(employee, "GET", mediaPath, nil, "", map[string]string{"Range": "bytes=0-9"}, 206)
	if !bytes.Equal(data, picture[:10]) || response.Header.Get("Content-Type") != "image/png" || response.Header.Get("Content-Range") != "bytes 0-9/"+strconv.Itoa(len(picture)) || response.Header.Get("X-Content-Type-Options") != "nosniff" || response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal("incorrect or unsafe ranged media delivery")
	}
	head, response := send(employee, "HEAD", mediaPath, nil, "", nil, 200)
	if len(head) != 0 || response.ContentLength != int64(len(picture)) || response.Header.Get("Content-Type") != "image/png" {
		t.Fatal("HEAD media response incorrect")
	}
	call(anonymous, "GET", mediaPath, nil, 401)
	var views knowledge.ViewResult
	decode(call(employee, "POST", articlePath+"/views", map[string]any{}, 200), &views)
	if !views.Recorded || views.ViewCount != 1 {
		t.Fatal("view event missing")
	}
	decode(call(employee, "POST", articlePath+"/views", map[string]any{}, 200), &views)
	if views.Recorded || views.ViewCount != 1 {
		t.Fatal("view retry was double counted")
	}
	feedbackInput := map[string]any{"attempt_id": employeeID, "helpful": true}
	var feedback knowledge.FeedbackResult
	decode(call(employee, "POST", articlePath+"/feedback", feedbackInput, 200), &feedback)
	if !feedback.Recorded || feedback.HelpfulCount != 1 {
		t.Fatal("helpful event missing")
	}
	decode(call(employee, "POST", articlePath+"/feedback", feedbackInput, 200), &feedback)
	if feedback.Recorded || feedback.HelpfulCount != 1 {
		t.Fatal("feedback retry was double counted")
	}
	feedbackInput["helpful"] = false
	call(employee, "POST", articlePath+"/feedback", feedbackInput, 409)
	input["version"] = 1
	call(admin, "PATCH", articlePath, input, 409)
	categoryInput["is_active"] = false
	call(admin, "PATCH", "/knowledge/categories/"+categoryID, categoryInput, 200)
	call(employee, "GET", articlePath, nil, 404)
	call(employee, "GET", mediaPath, nil, 404)
	call(employee, "POST", articlePath+"/views", map[string]any{}, 404)
	call(employee, "POST", articlePath+"/feedback", feedbackInput, 404)
	send(admin, "GET", mediaPath, nil, "", nil, 200) // Own upload remains privately accessible.
	categoryInput["is_active"] = true
	call(admin, "PATCH", "/knowledge/categories/"+categoryID, categoryInput, 200)
	call(employee, "GET", articlePath, nil, 200)
	input["version"] = articleResult.Article.Version
	input["is_published"] = false
	decode(call(admin, "PATCH", articlePath, input, 200), &articleResult)
	call(employee, "GET", articlePath, nil, 404)
	call(employee, "GET", mediaPath, nil, 404)
	call(admin, "DELETE", articlePath, map[string]any{"version": articleResult.Article.Version}, 204)
	call(employee, "GET", mediaPath, nil, 404)
	call(admin, "GET", articlePath, nil, 404)
}
