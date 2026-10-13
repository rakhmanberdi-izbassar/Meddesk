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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/media"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/health"
	"github.com/meddesk/meddesk/backend/internal/tickets"
)

type ticketBrowser struct {
	client *http.Client
	csrf   string
	user   auth.User
}

type ticketHTTPFixture struct {
	ctx                context.Context
	pool               *pgxpool.Pool
	server             *httptest.Server
	admin, employee    *ticketBrowser
	other, engineer    *ticketBrowser
	secondEngineer     *ticketBrowser
	anonymous          *ticketBrowser
	category, article  string
	inactiveEngineerID string
}

// Each fixture uses authFixture's TEST_DATABASE_URL gate, creates synthetic
// accounts, and removes all committed data before that fixture removes its
// department. It never reads the application's environment or database URL.
func newTicketHTTPFixture(t *testing.T) *ticketHTTPFixture {
	t.Helper()
	_, pool, authentication, _, email, password := authFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	f := &ticketHTTPFixture{ctx: ctx, pool: pool}
	var adminID, departmentID string
	if err := pool.QueryRow(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='ADMIN') WHERE email=$1 RETURNING id::text,department_id::text`, email).Scan(&adminID, &departmentID); err != nil {
		t.Fatal(err)
	}
	userIDs := []string{adminID}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, operation := range []struct {
			name, sql string
			args      []any
		}{
			{"tickets", `DELETE FROM tickets WHERE department_id=$1::uuid`, []any{departmentID}},
			{"articles", `DELETE FROM knowledge_articles WHERE author_id=ANY($1::uuid[])`, []any{userIDs}},
			{"media", `DELETE FROM media_files WHERE uploader_id=ANY($1::uuid[])`, []any{userIDs}},
			{"category", `DELETE FROM knowledge_categories WHERE id=$1::uuid`, []any{f.category}},
			{"audit", `DELETE FROM audit_logs WHERE actor_id=ANY($1::uuid[]) OR entity_id=ANY($1::uuid[])`, []any{userIDs}},
			{"accounts", `DELETE FROM users WHERE id=ANY($1::uuid[]) AND id<>$2::uuid`, []any{userIDs, adminID}},
		} {
			if operation.name == "category" && f.category == "" {
				continue
			}
			if _, err := pool.Exec(clean, operation.sql, operation.args...); err != nil {
				t.Errorf("unable to remove synthetic ticket %s: %v", operation.name, err)
			}
		}
	})
	newBrowser := func() *ticketBrowser {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &ticketBrowser{client: &http.Client{Jar: jar, Timeout: 45 * time.Second}}
	}
	f.admin, f.employee, f.other = newBrowser(), newBrowser(), newBrowser()
	f.engineer, f.secondEngineer, f.anonymous = newBrowser(), newBrowser(), newBrowser()
	addresses := map[*ticketBrowser]string{f.admin: email}
	for _, account := range []struct {
		browser *ticketBrowser
		role    string
	}{
		{f.employee, auth.RoleEmployee}, {f.other, auth.RoleEmployee},
		{f.engineer, auth.RoleEngineer}, {f.secondEngineer, auth.RoleEngineer},
	} {
		var id, address string
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,display_name,password_hash,role_id,department_id) SELECT gen_random_uuid()::text || '@example.invalid','Synthetic HTTP ticket user',password_hash,(SELECT id FROM roles WHERE code=$2),department_id FROM users WHERE id=$1::uuid RETURNING id::text,email`, adminID, account.role).Scan(&id, &address); err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, id)
		addresses[account.browser] = address
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,display_name,password_hash,role_id,department_id,is_active) SELECT gen_random_uuid()::text || '@example.invalid','Synthetic inactive engineer',password_hash,(SELECT id FROM roles WHERE code='IT_ENGINEER'),department_id,false FROM users WHERE id=$1::uuid RETURNING id::text`, adminID).Scan(&f.inactiveEngineerID); err != nil {
		t.Fatal(err)
	}
	userIDs = append(userIDs, f.inactiveEngineerID)
	if err := pool.QueryRow(ctx, `INSERT INTO knowledge_categories(slug,name_kk,name_ru) VALUES($1,'HTTP санаты','HTTP категория') RETURNING id::text`, "tickets-"+adminID).Scan(&f.category); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO knowledge_articles(slug,title,description,category_id,symptoms,solution_steps,author_id,is_published,published_at) VALUES($1,'Synthetic published ticket guide','Synthetic description',$2::uuid,'Synthetic symptoms','["Check the synthetic device"]',$3::uuid,true,clock_timestamp()) RETURNING id::text`, "tickets-"+adminID, f.category, adminID).Scan(&f.article); err != nil {
		t.Fatal(err)
	}
	files, err := media.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = files.Close() })
	options := auth.Options{AllowedOrigins: []string{"http://meddesk.test"}, SessionTTL: 8 * time.Hour}
	guard := auth.NewGuard(authentication, options)
	authHandler := auth.NewHandler(authentication, guard, options)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(pool))
	knowledgeHandler := knowledge.NewHandler(knowledgeService, guard)
	ticketHandler := tickets.NewHandler(tickets.NewService(tickets.NewRepository(pool), knowledgeService), guard)
	mediaHandler := media.NewHandler(media.NewService(media.NewRepository(pool), files, media.Options{}), guard)
	// Real bcrypt comparisons under -race need the same portable allowance as
	// the other multi-login HTTP integration fixtures, without changing the
	// production request timeout.
	router := health.NewHandler(health.NewService(pool, db.New(pool)), "test").RouterWithMedia(slog.New(slog.NewJSONHandler(io.Discard, nil)), 30*time.Second, 2*time.Minute, 5*time.Minute, authHandler.RegisterRoutes, knowledgeHandler.RegisterRoutes, ticketHandler.RegisterRoutes, mediaHandler.RegisterRoutes)
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	for _, browser := range []*ticketBrowser{f.admin, f.employee, f.other, f.engineer, f.secondEngineer} {
		var session struct {
			User auth.User `json:"user"`
			CSRF string    `json:"csrf_token"`
		}
		decodeTicketJSON(t, f.call(t, browser, "POST", "/auth/login", map[string]string{"email": addresses[browser], "password": password}, 200), &session)
		if session.User.ID == "" || session.CSRF == "" {
			t.Fatal("ticket HTTP fixture login returned an incomplete session")
		}
		browser.user, browser.csrf = session.User, session.CSRF
	}
	return f
}

func (f *ticketHTTPFixture) request(browser *ticketBrowser, method, path string, payload []byte, contentType string, headers map[string]string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(f.ctx, method, f.server.URL+"/api/v1"+path, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Origin", "http://meddesk.test")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if browser.csrf != "" {
		request.Header.Set(auth.CSRFHeader, browser.csrf)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := browser.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	return data, response.StatusCode, err
}

func (f *ticketHTTPFixture) call(t *testing.T, browser *ticketBrowser, method, path string, body any, want int) []byte {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	data, status, err := f.request(browser, method, path, payload, "application/json", nil)
	if err != nil {
		t.Fatalf("%s %s request failed: %v", method, path, err)
	}
	if status != want {
		t.Fatalf("%s %s status=%d expected=%d response=%s", method, path, status, want, data)
	}
	return data
}

func decodeTicketJSON(t *testing.T, data []byte, destination any) {
	t.Helper()
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("unexpected ticket response shape: %v", err)
	}
}

func (f *ticketHTTPFixture) upload(t *testing.T, browser *ticketBrowser, purpose string, want int) string {
	t.Helper()
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="synthetic-ticket.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(mediaPNG(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, status, err := f.request(browser, "POST", "/media?purpose="+purpose, payload.Bytes(), writer.FormDataContentType(), nil)
	if err != nil || status != want {
		t.Fatalf("ticket attachment upload status=%d expected=%d error=%v response=%s", status, want, err, data)
	}
	if want != 201 {
		return ""
	}
	var result struct {
		Media media.File `json:"media"`
	}
	decodeTicketJSON(t, data, &result)
	if result.Media.ID == "" || result.Media.Purpose != purpose || bytes.Contains(data, []byte("storage_key")) {
		t.Fatal("ticket media upload returned invalid or private metadata")
	}
	return result.Media.ID
}

func ticketFromResponse(t *testing.T, data []byte) tickets.Ticket {
	t.Helper()
	var result struct {
		Ticket tickets.Ticket `json:"ticket"`
	}
	decodeTicketJSON(t, data, &result)
	if result.Ticket.ID == "" || result.Ticket.Version < 1 {
		t.Fatal("ticket mutation response is incomplete")
	}
	return result.Ticket
}

// This crosses the HTTP guard, PostgreSQL transactions and private media store
// as three roles. A requester must never learn another requester's issue or an
// internal engineering note, even by reusing identifiers, filters or headers.
func TestTicketsHTTPAuthorizationAttachmentsAndResolutionWorkflow(t *testing.T) {
	f := newTicketHTTPFixture(t)
	f.call(t, f.anonymous, "GET", "/tickets", nil, 401)
	input := tickets.CreateInput{Title: "HTTP ticket " + f.employee.user.ID, Description: "Synthetic printer issue", Priority: "HIGH", CategoryID: &f.category, ArticleIDs: []string{f.article}, AttachmentIDs: []string{}}
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	_, status, err := f.request(f.employee, "POST", "/tickets", payload, "application/json", map[string]string{auth.CSRFHeader: ""})
	if err != nil || status != 403 {
		t.Fatalf("ticket creation without CSRF status=%d error=%v", status, err)
	}
	attachmentID := f.upload(t, f.employee, media.PurposeTicket, 201)
	foreignAttachment := f.upload(t, f.other, media.PurposeTicket, 201)
	wrongPurpose := f.upload(t, f.admin, media.PurposeImage, 201)
	f.upload(t, f.employee, media.PurposeImage, 403)
	data, status, err := f.request(f.employee, "GET", "/media/"+attachmentID, nil, "", nil)
	if err != nil || status != 200 || !bytes.Equal(data, mediaPNG(t)) {
		t.Fatalf("requester cannot download own upload: status=%d error=%v", status, err)
	}
	for _, browser := range []*ticketBrowser{f.other, f.engineer} {
		_, status, err := f.request(browser, "GET", "/media/"+attachmentID, nil, "", map[string]string{"Range": "bytes=0-9", "If-None-Match": "*"})
		if err != nil || status != 404 {
			t.Fatalf("unlinked screenshot leaked through range/conditional request: status=%d error=%v", status, err)
		}
	}
	input.AttachmentIDs = []string{foreignAttachment}
	f.call(t, f.employee, "POST", "/tickets", input, 403)
	input.AttachmentIDs = []string{wrongPurpose}
	f.call(t, f.employee, "POST", "/tickets", input, 403)
	input.AttachmentIDs = []string{attachmentID, attachmentID}
	f.call(t, f.employee, "POST", "/tickets", input, 400)
	input.AttachmentIDs = []string{attachmentID}
	issue := ticketFromResponse(t, f.call(t, f.employee, "POST", "/tickets", input, 201))
	path := "/tickets/" + issue.ID
	if issue.Status != tickets.New || issue.Version != 1 || issue.RequesterID != f.employee.user.ID || issue.DepartmentID != f.employee.user.DepartmentID || issue.Number < 1 || len(issue.ArticleIDs) != 1 || issue.ArticleIDs[0] != f.article || len(issue.AttachmentIDs) != 1 || issue.AttachmentIDs[0] != attachmentID {
		t.Fatalf("created ticket did not preserve requester and selected context: %+v", issue)
	}
	otherInput := input
	otherInput.AttachmentIDs = []string{foreignAttachment}
	f.call(t, f.other, "POST", "/tickets", otherInput, 201)
	for _, actor := range []struct {
		browser *ticketBrowser
		want    int64
	}{
		{f.employee, 1}, {f.other, 1}, {f.engineer, 2}, {f.admin, 2},
	} {
		var page tickets.Page
		decodeTicketJSON(t, f.call(t, actor.browser, "GET", "/tickets?q="+f.employee.user.ID+"&department_id="+f.employee.user.DepartmentID, nil, 200), &page)
		if page.Total != actor.want || int64(len(page.Items)) != actor.want {
			t.Fatalf("role %s list leaked or omitted tickets: total=%d items=%d", actor.browser.user.Role, page.Total, len(page.Items))
		}
		for _, item := range page.Items {
			if actor.browser.user.Role == auth.RoleEmployee && item.RequesterID != actor.browser.user.ID {
				t.Fatal("employee list included another requester's ticket")
			}
		}
	}
	f.call(t, f.other, "GET", path, nil, 404)
	f.call(t, f.other, "POST", path+"/comments", tickets.CommentInput{Version: issue.Version, Body: "Must stay private"}, 404)
	f.call(t, f.other, "POST", path+"/attachments", tickets.AttachmentInput{Version: issue.Version, AttachmentIDs: []string{foreignAttachment}}, 404)
	f.call(t, f.other, "POST", path+"/confirm", tickets.VersionInput{Version: issue.Version}, 404)
	for _, headers := range []map[string]string{{"Range": "bytes=0-9"}, {"If-None-Match": "*"}, {"Range": "bytes=0-9", "If-Modified-Since": time.Now().Add(time.Hour).Format(http.TimeFormat)}} {
		_, status, err := f.request(f.other, "GET", "/media/"+attachmentID, nil, "", headers)
		if err != nil || status != 404 {
			t.Fatalf("another requester's attached screenshot leaked through request headers: status=%d error=%v", status, err)
		}
	}
	f.call(t, f.engineer, "GET", "/media/"+attachmentID, nil, 200)
	f.call(t, f.employee, "PATCH", path, map[string]any{"version": issue.Version, "priority": "CRITICAL"}, 403)
	f.call(t, f.employee, "POST", path+"/comments", tickets.CommentInput{Version: issue.Version, Body: "Must not become an internal note", IsInternal: true}, 403)
	f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "status": tickets.Closed}, 409)
	for _, invalidAssignee := range []string{f.employee.user.ID, f.inactiveEngineerID} {
		f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "assignee_id": invalidAssignee}, 403)
	}
	var assignees struct {
		Items []tickets.Assignee `json:"items"`
	}
	decodeTicketJSON(t, f.call(t, f.engineer, "GET", "/tickets/assignees", nil, 200), &assignees)
	foundEngineer := false
	for _, assignee := range assignees.Items {
		if assignee.ID == f.inactiveEngineerID || assignee.ID == f.employee.user.ID {
			t.Fatal("assignee directory exposed an ineligible account")
		}
		foundEngineer = foundEngineer || assignee.ID == f.engineer.user.ID
	}
	if !foundEngineer {
		t.Fatal("active engineer missing from assignee directory")
	}
	f.call(t, f.employee, "GET", "/tickets/assignees", nil, 403)
	const internalNote = "Synthetic private infrastructure diagnosis"
	issue = ticketFromResponse(t, f.call(t, f.engineer, "POST", path+"/comments", tickets.CommentInput{Version: issue.Version, Body: internalNote, IsInternal: true}, 200))
	issue = ticketFromResponse(t, f.call(t, f.employee, "POST", path+"/comments", tickets.CommentInput{Version: issue.Version, Body: "Synthetic employee follow-up"}, 200))
	for _, actor := range []struct {
		browser *ticketBrowser
		count   int
	}{
		{f.employee, 1}, {f.engineer, 2}, {f.admin, 2},
	} {
		data := f.call(t, actor.browser, "GET", path, nil, 200)
		var detail tickets.Detail
		decodeTicketJSON(t, data, &detail)
		if len(detail.Comments) != actor.count || len(detail.History) != 1 || detail.History[0].ToStatus != tickets.New || detail.History[0].FromStatus != nil {
			t.Fatalf("ticket detail has wrong comment/history visibility: comments=%d history=%+v", len(detail.Comments), detail.History)
		}
		if actor.browser == f.employee && bytes.Contains(data, []byte(internalNote)) {
			t.Fatal("employee detail leaked internal comment text")
		}
	}
	f.call(t, f.employee, "POST", path+"/comments", tickets.CommentInput{Version: 1, Body: "Stale reply"}, 409)
	f.call(t, f.employee, "POST", path+"/attachments", tickets.AttachmentInput{Version: issue.Version, AttachmentIDs: []string{foreignAttachment}}, 403)
	f.call(t, f.admin, "POST", path+"/attachments", tickets.AttachmentInput{Version: issue.Version, AttachmentIDs: []string{wrongPurpose}}, 403)
	extraAttachment := f.upload(t, f.employee, media.PurposeTicket, 201)
	issue = ticketFromResponse(t, f.call(t, f.employee, "POST", path+"/attachments", tickets.AttachmentInput{Version: issue.Version, AttachmentIDs: []string{extraAttachment}}, 200))
	if len(issue.AttachmentIDs) != 2 {
		t.Fatal("additive ticket attachment operation lost an existing screenshot")
	}
	issue = ticketFromResponse(t, f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "assignee_id": f.engineer.user.ID, "status": tickets.InProgress}, 200))
	if issue.AssigneeID == nil || *issue.AssigneeID != f.engineer.user.ID || issue.ResolvedAt != nil {
		t.Fatal("assignment/start state did not persist")
	}
	for _, next := range []string{tickets.Waiting, tickets.InProgress, tickets.Resolved} {
		issue = ticketFromResponse(t, f.call(t, f.secondEngineer, "PATCH", path, map[string]any{"version": issue.Version, "status": next, "reason": "Synthetic workflow transition"}, 200))
	}
	if issue.ResolvedAt == nil || issue.ResolvedAt.Before(issue.CreatedAt) {
		t.Fatal("resolved ticket has no valid resolution timestamp")
	}
	f.call(t, f.engineer, "POST", path+"/confirm", tickets.VersionInput{Version: issue.Version}, 403)
	issue = ticketFromResponse(t, f.call(t, f.employee, "POST", path+"/confirm", tickets.VersionInput{Version: issue.Version}, 200))
	if issue.Status != tickets.Closed || issue.ResolvedAt == nil {
		t.Fatal("requester confirmation did not close the resolved ticket")
	}
	f.call(t, f.employee, "POST", path+"/comments", tickets.CommentInput{Version: issue.Version, Body: "Closed issue reply"}, 409)
	f.call(t, f.employee, "POST", path+"/attachments", tickets.AttachmentInput{Version: issue.Version, AttachmentIDs: []string{extraAttachment}}, 409)
	f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "priority": "LOW"}, 409)
	f.call(t, f.engineer, "POST", path+"/reopen", tickets.ReopenInput{Version: issue.Version, Reason: "Must require administrator"}, 403)
	f.call(t, f.employee, "POST", path+"/reopen", tickets.ReopenInput{Version: issue.Version, Reason: "Must require administrator"}, 403)
	issue = ticketFromResponse(t, f.call(t, f.admin, "POST", path+"/reopen", tickets.ReopenInput{Version: issue.Version, Reason: "Administrator confirms the issue returned"}, 200))
	if issue.Status != tickets.InProgress || issue.ResolvedAt != nil {
		t.Fatal("administrator reopen did not clear the resolution timestamp")
	}
	issue = ticketFromResponse(t, f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "status": tickets.Resolved}, 200))
	conversion := tickets.ArticleInput{Version: issue.Version, Slug: "converted-" + issue.ID, Title: "Synthetic resolved printer guide", Description: "A synthetic resolution", CategoryID: f.category, Symptoms: "Synthetic printer failure", SolutionSteps: []string{"Check the synthetic cable", "Retry the synthetic printer"}, Tags: []string{}}
	f.call(t, f.employee, "POST", path+"/article", conversion, 403)
	var converted struct {
		Article knowledge.Article `json:"article"`
	}
	decodeTicketJSON(t, f.call(t, f.engineer, "POST", path+"/article", conversion, 201), &converted)
	if converted.Article.ID == "" || converted.Article.IsPublished || converted.Article.PublishedAt != nil {
		t.Fatal("ticket conversion did not produce an unpublished knowledge draft")
	}
	var detail tickets.Detail
	decodeTicketJSON(t, f.call(t, f.engineer, "GET", path, nil, 200), &detail)
	issue = detail.Ticket
	if issue.ConvertedArticleID == nil || *issue.ConvertedArticleID != converted.Article.ID {
		t.Fatal("converted article link was not persisted on the ticket")
	}
	conversion.Version = issue.Version
	conversion.Slug += "-retry"
	f.call(t, f.admin, "POST", path+"/article", conversion, 409)
	f.call(t, f.employee, "GET", "/knowledge/articles/"+converted.Article.ID, nil, 404)
	f.call(t, f.admin, "GET", "/knowledge/articles/"+converted.Article.ID, nil, 200)
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM knowledge_articles WHERE slug=$1 OR slug=$2`, "converted-"+issue.ID, conversion.Slug).Scan(&count); err != nil || count != 1 {
		t.Fatalf("repeat conversion created duplicate knowledge drafts: count=%d error=%v", count, err)
	}
	assertTicketConcurrentUpdate(t, f)
	// The cookie remains usable, but guard and repository authorization must use
	// the current role for both the ticket and its previously visible screenshot.
	if _, err := f.pool.Exec(f.ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='EMPLOYEE') WHERE id=$1::uuid`, f.engineer.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call(t, f.engineer, "GET", path, nil, 404)
	f.call(t, f.engineer, "POST", path+"/comments", tickets.CommentInput{Version: issue.Version, Body: "Stale engineer role"}, 404)
	_, status, err = f.request(f.engineer, "GET", "/media/"+attachmentID, nil, "", map[string]string{"Range": "bytes=0-9", "If-None-Match": "*"})
	if err != nil || status != 404 {
		t.Fatalf("role downgrade retained ticket screenshot access: status=%d error=%v", status, err)
	}
}

func assertTicketConcurrentUpdate(t *testing.T, f *ticketHTTPFixture) {
	t.Helper()
	issue := ticketFromResponse(t, f.call(t, f.admin, "POST", "/tickets", tickets.CreateInput{Title: "Synthetic concurrent ticket", Description: "Two engineers act on the same version", ArticleIDs: []string{}, AttachmentIDs: []string{}}, 201))
	path := "/tickets/" + issue.ID
	var beforeAudits int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_logs WHERE entity_id=$1::uuid`, issue.ID).Scan(&beforeAudits); err != nil {
		t.Fatal(err)
	}
	type result struct {
		status int
		err    error
	}
	start := make(chan struct{})
	done := make(chan result, 2)
	for _, priority := range []string{"LOW", "CRITICAL"} {
		payload, err := json.Marshal(map[string]any{"version": issue.Version, "priority": priority})
		if err != nil {
			t.Fatal(err)
		}
		go func(payload []byte) {
			<-start
			_, status, err := f.request(f.admin, "PATCH", path, payload, "application/json", nil)
			done <- result{status: status, err: err}
		}(payload)
	}
	close(start)
	statuses := map[int]int{}
	for range 2 {
		select {
		case result := <-done:
			if result.err != nil {
				t.Fatal(result.err)
			}
			statuses[result.status]++
		case <-f.ctx.Done():
			t.Fatal("concurrent ticket updates did not finish")
		}
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("same-version updates must have one winner: %v", statuses)
	}
	var version, audits, history int
	var priority string
	if err := f.pool.QueryRow(f.ctx, `SELECT version,priority,(SELECT count(*) FROM audit_logs WHERE entity_id=t.id),(SELECT count(*) FROM ticket_status_history WHERE ticket_id=t.id) FROM tickets t WHERE id=$1::uuid`, issue.ID).Scan(&version, &priority, &audits, &history); err != nil {
		t.Fatal(err)
	}
	if version != 2 || (priority != "LOW" && priority != "CRITICAL") || audits != beforeAudits+1 || history != 1 {
		t.Fatalf("concurrent loser left a mutation behind: version=%d priority=%s audits=%d history=%d", version, priority, audits, history)
	}
}

// Inject failures inside a surrounding test transaction, so failure triggers
// cannot affect other test connections. The repository's nested transactions
// must undo the domain write when either its history or audit write fails.
func TestTicketsRepositoryAuditAndHistoryFailuresRollBackEntireMutation(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, fixture := managementAdmin(t, ctx, tx)
	service := tickets.NewService(tickets.NewRepository(tx), knowledge.NewService(knowledge.NewRepository(tx)))
	input := tickets.CreateInput{Title: "Synthetic transactional ticket", Description: "Every domain and audit write must commit together", CategoryID: &fixture.category, ArticleIDs: []string{}, AttachmentIDs: []string{}}
	issue, err := service.Create(ctx, actor, input)
	if err != nil {
		t.Fatal(err)
	}
	mediaID := knowledgeMediaFixture(t, ctx, tx, actor.ID, media.PurposeTicket)
	var beforeAudits, beforeHistory int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE entity_id=$1::uuid),(SELECT count(*) FROM ticket_status_history WHERE ticket_id=$1::uuid)`, issue.ID).Scan(&beforeAudits, &beforeHistory); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `CREATE FUNCTION pg_temp.reject_ticket_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic ticket transaction failure'; END $$`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_ticket_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_ticket_write()`)
	status := tickets.InProgress
	if _, err := service.Update(ctx, actor, issue.ID, tickets.UpdateInput{Version: issue.Version, Status: &status}); err == nil {
		t.Fatal("status update succeeded without its audit")
	}
	if _, err := service.Comment(ctx, actor, issue.ID, tickets.CommentInput{Version: issue.Version, Body: "Must roll back with audit", IsInternal: true}); err == nil {
		t.Fatal("comment succeeded without its audit")
	}
	if _, err := service.Attach(ctx, actor, issue.ID, tickets.AttachmentInput{Version: issue.Version, AttachmentIDs: []string{mediaID}}); err == nil {
		t.Fatal("attachment succeeded without its audit")
	}
	if _, err := service.Create(ctx, actor, input); err == nil {
		t.Fatal("ticket creation succeeded without its audit")
	}
	assertUnchanged := func() {
		t.Helper()
		stored, err := service.Get(ctx, actor, issue.ID)
		if err != nil {
			t.Fatal(err)
		}
		var audits, history, created int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE entity_id=$1::uuid),(SELECT count(*) FROM ticket_status_history WHERE ticket_id=$1::uuid),(SELECT count(*) FROM tickets WHERE title=$2)`, issue.ID, input.Title).Scan(&audits, &history, &created); err != nil {
			t.Fatal(err)
		}
		if stored.Ticket.Version != issue.Version || stored.Ticket.Status != tickets.New || stored.Ticket.ResolvedAt != nil || len(stored.Comments) != 0 || len(stored.Ticket.AttachmentIDs) != 0 || audits != beforeAudits || history != beforeHistory || created != 1 {
			t.Fatalf("failed transaction left domain/history/audit changes: ticket=%+v comments=%d audits=%d history=%d created=%d", stored.Ticket, len(stored.Comments), audits, history, created)
		}
	}
	assertUnchanged()
	execSQL(t, ctx, tx, `DROP TRIGGER reject_ticket_audit ON audit_logs`)
	issue, err = service.Update(ctx, actor, issue.ID, tickets.UpdateInput{Version: issue.Version, Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	status = tickets.Resolved
	issue, err = service.Update(ctx, actor, issue.ID, tickets.UpdateInput{Version: issue.Version, Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE entity_id=$1::uuid),(SELECT count(*) FROM ticket_status_history WHERE ticket_id=$1::uuid)`, issue.ID).Scan(&beforeAudits, &beforeHistory); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_ticket_history BEFORE INSERT ON ticket_status_history FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_ticket_write()`)
	if _, err := service.Confirm(ctx, actor, issue.ID, tickets.VersionInput{Version: issue.Version}); err == nil {
		t.Fatal("requester confirmation succeeded without its status history")
	}
	resolved, err := service.Get(ctx, actor, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var audits, history int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE entity_id=$1::uuid),(SELECT count(*) FROM ticket_status_history WHERE ticket_id=$1::uuid)`, issue.ID).Scan(&audits, &history); err != nil {
		t.Fatal(err)
	}
	if resolved.Ticket.Version != issue.Version || resolved.Ticket.Status != tickets.Resolved || resolved.Ticket.ResolvedAt == nil || !resolved.Ticket.ResolvedAt.Equal(*issue.ResolvedAt) || audits != beforeAudits || history != beforeHistory {
		t.Fatal("history failure partially closed the ticket or changed its audit")
	}
	execSQL(t, ctx, tx, `DROP TRIGGER reject_ticket_history ON ticket_status_history`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_ticket_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_ticket_write()`)
	conversion := tickets.ArticleInput{Version: issue.Version, Slug: "rollback-" + issue.ID, Title: "Must not survive failed conversion", Description: "Synthetic guide", CategoryID: fixture.category, Symptoms: "Synthetic failure", SolutionSteps: []string{"Synthetic fix"}, Tags: []string{}}
	if _, err := service.Convert(ctx, actor, issue.ID, conversion); err == nil {
		t.Fatal("ticket conversion succeeded without its audit")
	}
	stored, err := service.Get(ctx, actor, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var articles, links int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM knowledge_articles WHERE slug=$1),(SELECT count(*) FROM ticket_articles WHERE ticket_id=$2::uuid)`, conversion.Slug, issue.ID).Scan(&articles, &links); err != nil {
		t.Fatal(err)
	}
	if articles != 0 || links != 0 || stored.Ticket.ConvertedArticleID != nil || stored.Ticket.Version != issue.Version {
		t.Fatalf("failed conversion was not atomic: articles=%d links=%d version=%d", articles, links, stored.Ticket.Version)
	}
}
