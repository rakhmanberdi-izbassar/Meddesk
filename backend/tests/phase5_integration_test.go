package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/audit"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/health"
	"github.com/meddesk/meddesk/backend/internal/settings"
	"github.com/meddesk/meddesk/backend/internal/tickets"
)

func phase5Float(t *testing.T, got *float64, want float64, label string) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 0.000001 {
		t.Fatalf("%s: got%v want%v", label, got, want)
	}
}
func phase5String(value string) *string { return &value }
func TestPhase5AnalyticsExactCohortsTimezoneFiltersAndIndependentEventAggregates(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := analytics.NewService(analytics.NewRepository(tx))
	var otherDepartment, otherUser, otherCategory, otherArticle string
	queryID(t, ctx, tx, &otherDepartment, `INSERT INTO departments(name) VALUES('Reporting B '||gen_random_uuid()) RETURNING id::text`)
	queryID(t, ctx, tx, &otherUser, `INSERT INTO users(email,display_name,password_hash,role_id,department_id) SELECT gen_random_uuid()::text||'@example.invalid','Reporting second engineer',password_hash,(SELECT id FROM roles WHERE code='IT_ENGINEER'),$2::uuid FROM users WHERE id=$1::uuid RETURNING id::text`, actor.ID, otherDepartment)
	queryID(t, ctx, tx, &otherCategory, `INSERT INTO knowledge_categories(slug,name_kk,name_ru)VALUES('reporting-'||gen_random_uuid(),'Екінші','Вторая')RETURNING id::text`)
	queryID(t, ctx, tx, &otherArticle, `INSERT INTO knowledge_articles(slug,title,description,category_id,symptoms,solution_steps,author_id)VALUES('reporting-'||gen_random_uuid(),'Reporting article B','Summary',$1::uuid,'Symptoms','["Synthetic step"]',$2::uuid)RETURNING id::text`, otherCategory, actor.ID)
	execSQL(t, ctx, tx, `UPDATE tickets SET created_at='2025-01-14T19:00:00Z',category_id=$2::uuid WHERE id=$1::uuid`, f.ticket, f.category)
	insertTicket := func(department, category, assignee, status, created string, hours int) string {
		t.Helper()
		var id string
		queryID(t, ctx, tx, &id, `INSERT INTO tickets(title,description,requester_id,department_id,category_id,assignee_id,status,created_at,resolved_at) VALUES('Synthetic reporting cohort','Reporting descriptions stay out of analytics',$1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6::timestamptz,CASE WHEN $5 IN ('RESOLVED','CLOSED') THEN $6::timestamptz+$7::int*interval '1 hour' ELSE NULL END)RETURNING id::text`, actor.ID, department, category, assignee, status, created, hours)
		return id
	}
	resolved := insertTicket(f.department, f.category, actor.ID, tickets.Resolved, "2025-01-15T10:00:00Z", 1)
	closed := insertTicket(f.department, f.category, actor.ID, tickets.Closed, "2025-01-15T11:00:00Z", 3)
	reopened := insertTicket(f.department, f.category, otherUser, tickets.InProgress, "2025-01-15T12:00:00Z", 0)
	insertTicket(f.department, f.category, actor.ID, tickets.New, "2025-01-15T19:00:00Z", 0)
	insertTicket(f.department, f.category, actor.ID, tickets.New, "2025-01-14T18:59:59Z", 0)
	insertTicket(otherDepartment, otherCategory, otherUser, tickets.New, "2025-01-15T09:00:00Z", 0)
	execSQL(t, ctx, tx, `INSERT INTO ticket_status_history(ticket_id,from_status,to_status,actor_id,reason,created_at)VALUES($1::uuid,'RESOLVED','IN_PROGRESS',$2::uuid,'Returned','2025-01-15T15:00:00Z')`, reopened, actor.ID)
	// Extra associations/comments/history must never multiply ticket or event counts.
	for _, id := range []string{resolved, closed, reopened} {
		execSQL(t, ctx, tx, `INSERT INTO ticket_articles(ticket_id,article_id)VALUES($1::uuid,$2::uuid),($1::uuid,$3::uuid)`, id, f.article, otherArticle)
		execSQL(t, ctx, tx, `INSERT INTO ticket_comments(ticket_id,author_id,body)VALUES($1::uuid,$2::uuid,'One'),($1::uuid,$2::uuid,'Two')`, id, actor.ID)
	}
	for _, e := range []struct {
		article, user string
		helpful       bool
		at            string
	}{{f.article, actor.ID, true, "2025-01-14T19:00:00Z"}, {f.article, actor.ID, true, "2025-01-15T10:00:00Z"}, {f.article, actor.ID, false, "2025-01-15T11:00:00Z"}, {otherArticle, otherUser, false, "2025-01-15T12:00:00Z"}, {f.article, actor.ID, true, "2025-01-15T19:00:00Z"}} {
		execSQL(t, ctx, tx, `INSERT INTO article_feedback(article_id,user_id,attempt_id,helpful,created_at)VALUES($1::uuid,$2::uuid,gen_random_uuid(),$3,$4::timestamptz)`, e.article, e.user, e.helpful, e.at)
	}
	for _, e := range []struct{ article, user, at string }{{f.article, actor.ID, "2025-01-14T19:00:00Z"}, {f.article, actor.ID, "2025-01-15T10:00:00Z"}, {f.article, actor.ID, "2025-01-15T11:00:00Z"}, {otherArticle, otherUser, "2025-01-15T12:00:00Z"}, {f.article, actor.ID, "2025-01-15T19:00:00Z"}} {
		execSQL(t, ctx, tx, `INSERT INTO article_views(article_id,user_id,created_at)VALUES($1::uuid,$2::uuid,$3::timestamptz)`, e.article, e.user, e.at)
	}
	filter := analytics.Filter{From: "2025-01-15", To: "2025-01-15", Timezone: "Asia/Almaty", DepartmentID: f.department, CategoryID: f.category}
	report, err := service.Report(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summary
	if s.TicketsTotal != 4 || s.TicketsOpen != 2 || s.TicketsResolved != 2 || s.TicketsClosed != 1 || s.FeedbackAttempts != 3 || s.HelpfulAttempts != 2 {
		t.Fatalf("incorrect filtered cohort/events: %+v", s)
	}
	phase5Float(t, s.AverageResolutionSeconds, 7200, "resolution mean")
	phase5Float(t, s.SelfServiceRate, 2.0/3.0, "self-service ratio")
	if !report.Period.StartAt.Equal(time.Date(2025, 1, 14, 19, 0, 0, 0, time.UTC)) || !report.Period.EndAt.Equal(time.Date(2025, 1, 15, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("timezone interval wrong: %+v", report.Period)
	}
	if len(report.Departments) != 1 || report.Departments[0].ID != f.department || report.Departments[0].Total != 4 || len(report.Categories) != 1 || report.Categories[0].Total != 4 || len(report.CommonIssues) != 1 || report.CommonIssues[0].Total != 4 {
		t.Fatalf("incorrect breakdowns: %+v %+v %+v", report.Departments, report.Categories, report.CommonIssues)
	}
	if len(report.PopularArticles) != 1 || report.PopularArticles[0].ID != f.article || report.PopularArticles[0].ViewCount != 3 || report.PopularArticles[0].HelpfulCount != 2 || report.PopularArticles[0].NotHelpfulCount != 1 {
		t.Fatalf("event cross-join inflated popular article: %+v", report.PopularArticles)
	}
	if len(report.Trend) != 1 || report.Trend[0].Date != "2025-01-15" || report.Trend[0].TicketsCreated != 4 || report.Trend[0].TicketsResolved != 2 || report.Trend[0].FeedbackAttempts != 3 {
		t.Fatalf("trend cohort mismatch: %+v", report.Trend)
	}
	filter.AssigneeID = actor.ID
	assigned, err := service.Report(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Summary.TicketsTotal != 2 || assigned.Summary.TicketsOpen != 0 || assigned.Summary.FeedbackAttempts != 3 || assigned.Summary.HelpfulAttempts != 2 || len(assigned.PopularArticles) != 1 || assigned.PopularArticles[0].ViewCount != 3 {
		t.Fatalf("assignee filter must only affect tickets: %+v", assigned)
	}
	filter.DepartmentID = otherDepartment
	filter.CategoryID = otherCategory
	filter.AssigneeID = ""
	open, err := service.Report(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	if open.Summary.TicketsTotal != 1 || open.Summary.AverageResolutionSeconds != nil || open.Summary.FeedbackAttempts != 1 || open.Summary.HelpfulAttempts != 0 {
		t.Fatalf("open-only denominator semantics: %+v", open.Summary)
	}
	phase5Float(t, open.Summary.SelfServiceRate, 0, "unhelpful-only ratio")
	filter.From = "2025-02-01"
	filter.To = "2025-02-01"
	empty, err := service.Report(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Summary.TicketsTotal != 0 || empty.Summary.AverageResolutionSeconds != nil || empty.Summary.SelfServiceRate != nil || len(empty.Departments) != 0 || len(empty.Categories) != 0 || len(empty.PopularArticles) != 0 {
		t.Fatalf("empty report must keep undefined ratios null: %+v", empty)
	}
	// Feedback department follows the current user; ticket department remains its snapshot.
	execSQL(t, ctx, tx, `UPDATE users SET department_id=$2::uuid WHERE id=$1::uuid`, actor.ID, otherDepartment)
	filter = analytics.Filter{From: "2025-01-15", To: "2025-01-15", Timezone: "Asia/Almaty", DepartmentID: f.department}
	moved, err := service.Report(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Summary.TicketsTotal != 4 || moved.Summary.FeedbackAttempts != 0 || moved.Summary.SelfServiceRate != nil {
		t.Fatalf("historical ticket/current feedback department mismatch: %+v", moved.Summary)
	}
	// A stale administrator/engineer actor passed by the caller does not grant reports.
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='EMPLOYEE')WHERE id=$1::uuid`, actor.ID)
	if _, err := service.Report(ctx, actor, filter); !errors.Is(err, analytics.ErrForbidden) {
		t.Fatalf("stale staff role could read analytics: %v", err)
	}
	if _, err := service.Filters(ctx, actor); !errors.Is(err, analytics.ErrForbidden) {
		t.Fatalf("stale staff role could read report catalogs: %v", err)
	}
}

func TestPhase5AnalyticsCivilDayDSTAndDateValidation(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := analytics.NewService(analytics.NewRepository(tx))
	execSQL(t, ctx, tx, `UPDATE tickets SET created_at='2025-03-09T05:00:00Z' WHERE id=$1::uuid`, f.ticket)
	var excluded string
	queryID(t, ctx, tx, &excluded, `INSERT INTO tickets(title,description,requester_id,department_id,created_at)VALUES('DST exclusive end','Synthetic',$1::uuid,$2::uuid,'2025-03-10T04:00:00Z')RETURNING id::text`, actor.ID, f.department)
	report, err := service.Report(ctx, actor, analytics.Filter{From: "2025-03-09", To: "2025-03-09", Timezone: "America/New_York", DepartmentID: f.department})
	if err != nil {
		t.Fatal(err)
	}
	if report.Period.EndAt.Sub(report.Period.StartAt) != 23*time.Hour || report.Summary.TicketsTotal != 1 {
		t.Fatalf("DST civil day treated as fixed24hours: %+v", report)
	}
	for _, filter := range []analytics.Filter{{From: "2025-02-30", To: "2025-03-01"}, {From: "2025-03-01", To: "2025-02-28"}, {From: "2023-01-01", To: "2024-01-02"}, {From: "2025-01-01", To: "2025-01-01", Timezone: "Invalid/Timezone"}, {From: "2025-01-01", To: "2025-01-01", DepartmentID: "invalid"}} {
		if _, err := service.Report(ctx, actor, filter); !errors.Is(err, analytics.ErrInvalidInput) {
			t.Fatalf("invalid reporting filter accepted: %+v %v", filter, err)
		}
	}
}

func TestPhase5SettingsFreshAuthorizationVersionsAndAuditRollback(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, _ := managementAdmin(t, ctx, tx)
	service := settings.NewService(settings.NewRepository(tx))
	initial, err := service.Get(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.Update(ctx, actor, settings.UpdateInput{Version: initial.Version, ClinicName: phase5String("Synthetic clinic"), DefaultTicketPriority: phase5String("HIGH"), ReportingTimezone: phase5String("UTC")})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != initial.Version+1 || saved.ClinicName != "Synthetic clinic" || saved.DefaultTicketPriority != "HIGH" || saved.ReportingTimezone != "UTC" {
		t.Fatalf("settings not persisted: %+v", saved)
	}
	if _, err := service.Update(ctx, actor, settings.UpdateInput{Version: initial.Version, ClinicName: phase5String("Stale")}); !errors.Is(err, settings.ErrVersionConflict) {
		t.Fatalf("stale settings version accepted: %v", err)
	}
	var before int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='settings.updated'`, actor.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `CREATE FUNCTION pg_temp.reject_phase5_audit()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic phase5 audit failure'; END $$`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_phase5_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_phase5_audit()`)
	if _, err := service.Update(ctx, actor, settings.UpdateInput{Version: saved.Version, ClinicName: phase5String("Must roll back")}); err == nil {
		t.Fatal("settings update committed without audit")
	}
	after, err := service.Get(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='settings.updated'`, actor.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if after.Version != saved.Version || after.ClinicName != saved.ClinicName || auditCount != before {
		t.Fatalf("failed audit left partial settings: %+v count%d/%d", after, auditCount, before)
	}
	execSQL(t, ctx, tx, `DROP TRIGGER reject_phase5_audit ON audit_logs`)
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='IT_ENGINEER')WHERE id=$1::uuid`, actor.ID)
	if _, err := service.Update(ctx, actor, settings.UpdateInput{Version: saved.Version, ClinicName: phase5String("Forbidden")}); !errors.Is(err, settings.ErrForbidden) {
		t.Fatalf("stale administrator could edit settings: %v", err)
	}
	if _, err := service.Get(ctx, actor); err != nil {
		t.Fatalf("authenticated settings read forbidden: %v", err)
	}
	execSQL(t, ctx, tx, `UPDATE users SET is_active=false WHERE id=$1::uuid`, actor.ID)
	if _, err := service.Get(ctx, actor); !errors.Is(err, settings.ErrForbidden) {
		t.Fatalf("inactive actor read settings: %v", err)
	}
}

// Reuse the Phase4 multi-role synthetic browser sessions and recompose its
// server to include Phase5. Cookies belong to loopback rather than a TCP port.
func newPhase5HTTPFixture(t *testing.T) *ticketHTTPFixture {
	t.Helper()
	f := newTicketHTTPFixture(t)
	options := auth.Options{AllowedOrigins: []string{"http://meddesk.test"}, SessionTTL: 8 * time.Hour}
	authentication := auth.NewService(auth.NewRepository(f.pool), 8*time.Hour)
	guard := auth.NewGuard(authentication, options)
	authenticationHandler := auth.NewHandler(authentication, guard, options)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(f.pool))
	knowledgeHandler := knowledge.NewHandler(knowledgeService, guard)
	ticketHandler := tickets.NewHandler(tickets.NewService(tickets.NewRepository(f.pool), knowledgeService), guard)
	settingsHandler := settings.NewHandler(settings.NewService(settings.NewRepository(f.pool)), guard)
	analyticsHandler := analytics.NewHandler(analytics.NewService(analytics.NewRepository(f.pool)), guard)
	auditHandler := audit.NewHandler(audit.NewService(audit.NewRepository(f.pool)), guard)
	router := health.NewHandler(health.NewService(f.pool, db.New(f.pool)), "test").Router(slog.New(slog.NewJSONHandler(io.Discard, nil)), 30*time.Second, authenticationHandler.RegisterRoutes, knowledgeHandler.RegisterRoutes, ticketHandler.RegisterRoutes, settingsHandler.RegisterRoutes, analyticsHandler.RegisterRoutes, auditHandler.RegisterRoutes)
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}

func TestPhase5AuditFiltersPaginationFreshRoleAndLegacyMetadataScrubbing(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := audit.NewService(audit.NewRepository(tx))
	legacy := `{"version":7,"priority":"HIGH","status":"RESOLVED","password":"legacy-password","token":"legacy-token","cookie":"legacy-cookie","description":"legacy-body","title":"legacy-title","metadata":{"password":"nested-password"},"changed_fields":["clinic_name","password"],"assignee_id":"not-a-uuid","is_internal":{"token":"nested-token"},"byte_count":"secret-text","from_status":"secret-status","count":["hidden"]}`
	var first, second int64
	if err := tx.QueryRow(ctx, `INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,metadata,created_at)VALUES($1::uuid,'ticket.updated','ticket',$2::uuid,$3::jsonb,'2025-01-14T19:00:00Z')RETURNING id`, actor.ID, f.ticket, legacy).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,metadata,created_at)VALUES($1::uuid,'ticket.updated','ticket',$2::uuid,'{"version":8,"status":"CLOSED"}','2025-01-15T18:59:59Z')RETURNING id`, actor.ID, f.ticket).Scan(&second); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, tx, `INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,metadata,created_at)VALUES($1::uuid,'ticket.updated','ticket',$2::uuid,'{}','2025-01-15T19:00:00Z'),(NULL,'legacy.null_actor','ticket',NULL,'{}','2025-01-15T10:00:00Z')`, actor.ID, f.ticket)
	filter := audit.Filter{From: "2025-01-15", To: "2025-01-15", Timezone: "Asia/Almaty", ActorID: actor.ID, Action: "ticket.updated", EntityType: "ticket", EntityID: f.ticket, Limit: 1}
	page, err := service.List(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != second {
		t.Fatalf("audit order/count/filter wrong: %+v", page)
	}
	filter.Offset = 1
	page, err = service.List(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != first {
		t.Fatalf("audit pagination wrong: %+v", page)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"legacy-password", "legacy-token", "legacy-cookie", "legacy-body", "legacy-title", "nested-password", "nested-token", "not-a-uuid", "secret-text", "secret-status", "hidden", "password"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("audit response leaked unsafe legacy metadata field/value %q: %s", secret, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"version":7`) || !strings.Contains(string(encoded), `"priority":"HIGH"`) {
		t.Fatalf("safe typed metadata removed: %s", encoded)
	}
	filter.ActorID = ""
	filter.Action = "legacy.null_actor"
	filter.EntityID = ""
	filter.Offset = 0
	filter.Limit = 20
	page, err = service.List(ctx, actor, filter)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(page)
	if page.Total != 1 || !strings.Contains(string(encoded), `"actor_id":null`) || !strings.Contains(string(encoded), `"actor_name":null`) || !strings.Contains(string(encoded), `"entity_id":null`) {
		t.Fatalf("nullable audit references wrong: %s", encoded)
	}
	catalogs, err := service.Filters(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(catalogs)
	if strings.Contains(string(encoded), "@example.invalid") || strings.Contains(string(encoded), "password") {
		t.Fatalf("audit catalogs leaked account/legacy secrets: %s", encoded)
	}
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='IT_ENGINEER')WHERE id=$1::uuid`, actor.ID)
	if _, err := service.List(ctx, actor, filter); !errors.Is(err, audit.ErrForbidden) {
		t.Fatalf("stale administrator read audit: %v", err)
	}
	if _, err := service.Filters(ctx, actor); !errors.Is(err, audit.ErrForbidden) {
		t.Fatalf("stale administrator read audit catalogs: %v", err)
	}
}

func TestPhase5FullSelfServiceTicketKnowledgeReportingAndSettingsConcurrencyHTTP(t *testing.T) {
	f := newPhase5HTTPFixture(t)
	var initial struct {
		Settings settings.Settings `json:"settings"`
	}
	decodeTicketJSON(t, f.call(t, f.employee, "GET", "/settings", nil, 200), &initial)
	if initial.Settings.Version < 1 || initial.Settings.ClinicName == "" {
		t.Fatal("settings envelope missing")
	}
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.pool.Exec(clean, `UPDATE application_settings SET clinic_name=$1,default_ticket_priority=$2,reporting_timezone=$3,version=$4 WHERE singleton=true`, initial.Settings.ClinicName, initial.Settings.DefaultTicketPriority, initial.Settings.ReportingTimezone, initial.Settings.Version); err != nil {
			t.Errorf("restore disposable settings: %v", err)
		}
	})
	f.call(t, f.anonymous, "GET", "/settings", nil, 401)
	f.call(t, f.employee, "PATCH", "/settings", settings.UpdateInput{Version: initial.Settings.Version, ClinicName: phase5String("Forbidden")}, 403)
	f.call(t, f.engineer, "PATCH", "/settings", settings.UpdateInput{Version: initial.Settings.Version, ClinicName: phase5String("Forbidden")}, 403)
	var saved struct {
		Settings settings.Settings `json:"settings"`
	}
	decodeTicketJSON(t, f.call(t, f.admin, "PATCH", "/settings", settings.UpdateInput{Version: initial.Settings.Version, DefaultTicketPriority: phase5String("HIGH"), ReportingTimezone: phase5String("Asia/Almaty")}, 200), &saved)
	date := time.Now().In(func() *time.Location {
		v, err := time.LoadLocation("Asia/Almaty")
		if err != nil {
			t.Fatal(err)
		}
		return v
	}()).Format("2006-01-02")
	params := url.Values{"from": {date}, "to": {date}, "department_id": {f.employee.user.DepartmentID}, "category_id": {f.category}}
	reportPath := "/analytics?" + params.Encode()
	f.call(t, f.anonymous, "GET", reportPath, nil, 401)
	f.call(t, f.employee, "GET", reportPath, nil, 403)
	f.call(t, f.employee, "GET", "/analytics/filters", nil, 403)
	f.call(t, f.engineer, "GET", "/analytics/filters", nil, 200)
	f.call(t, f.employee, "GET", "/admin/audit", nil, 403)
	f.call(t, f.engineer, "GET", "/admin/audit/filters", nil, 403)
	var before analytics.Report
	decodeTicketJSON(t, f.call(t, f.engineer, "GET", reportPath, nil, 200), &before)
	if before.Summary.TicketsTotal != 0 {
		t.Fatal("fresh synthetic department has unexpected tickets")
	}
	var helpfulAttempt, failedAttempt string
	if err := f.pool.QueryRow(f.ctx, `SELECT gen_random_uuid()::text`).Scan(&helpfulAttempt); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT gen_random_uuid()::text`).Scan(&failedAttempt); err != nil {
		t.Fatal(err)
	}
	f.call(t, f.employee, "GET", "/tickets/suggestions?q=Synthetic&category_id="+f.category, nil, 200)
	f.call(t, f.employee, "POST", "/knowledge/articles/"+f.article+"/feedback", knowledge.FeedbackInput{AttemptID: helpfulAttempt, Helpful: boolValue(true)}, 200)
	var helped analytics.Report
	decodeTicketJSON(t, f.call(t, f.engineer, "GET", reportPath, nil, 200), &helped)
	if helped.Summary.TicketsTotal != 0 || helped.Summary.FeedbackAttempts != 1 || helped.Summary.HelpfulAttempts != 1 {
		t.Fatalf("helpful feedback should resolve without ticket: %+v", helped.Summary)
	}
	phase5Float(t, helped.Summary.SelfServiceRate, 1, "helpful-only HTTP ratio")
	// Matching immutable retry is a no-op; the unresolved attempt is independent.
	f.call(t, f.employee, "POST", "/knowledge/articles/"+f.article+"/feedback", knowledge.FeedbackInput{AttemptID: helpfulAttempt, Helpful: boolValue(true)}, 200)
	f.call(t, f.employee, "POST", "/knowledge/articles/"+f.article+"/feedback", knowledge.FeedbackInput{AttemptID: failedAttempt, Helpful: boolValue(false)}, 200)
	issue := ticketFromResponse(t, f.call(t, f.employee, "POST", "/tickets", tickets.CreateInput{Title: "Synthetic complete workflow", Description: "Guide did not solve this separate attempt", CategoryID: &f.category, ArticleIDs: []string{f.article}, AttachmentIDs: []string{}}, 201))
	if issue.Priority != "HIGH" {
		t.Fatalf("omitted ticket priority ignored saved default: %s", issue.Priority)
	}
	path := "/tickets/" + issue.ID
	issue = ticketFromResponse(t, f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "assignee_id": f.engineer.user.ID, "status": tickets.InProgress}, 200))
	issue = ticketFromResponse(t, f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "status": tickets.Resolved}, 200))
	issue = ticketFromResponse(t, f.call(t, f.employee, "POST", path+"/confirm", tickets.VersionInput{Version: issue.Version}, 200))
	var converted struct {
		Article knowledge.Article `json:"article"`
	}
	decodeTicketJSON(t, f.call(t, f.engineer, "POST", path+"/article", tickets.ArticleInput{Version: issue.Version, Slug: "phase5-" + issue.ID, Title: "Reviewed synthetic resolution", Description: "Safe guide", CategoryID: f.category, Symptoms: "Synthetic failure", SolutionSteps: []string{"Apply the reviewed fix"}, Tags: []string{}}, 201), &converted)
	if converted.Article.IsPublished || converted.Article.ID == "" {
		t.Fatal("resolved ticket draft should require review before publication")
	}
	f.call(t, f.employee, "GET", "/knowledge/articles/"+converted.Article.ID, nil, 404)
	var complete analytics.Report
	decodeTicketJSON(t, f.call(t, f.engineer, "GET", reportPath, nil, 200), &complete)
	if complete.Period.Timezone != "Asia/Almaty" || complete.Summary.TicketsTotal != 1 || complete.Summary.TicketsOpen != 0 || complete.Summary.TicketsResolved != 1 || complete.Summary.TicketsClosed != 1 || complete.Summary.FeedbackAttempts != 2 || complete.Summary.HelpfulAttempts != 1 || complete.Summary.AverageResolutionSeconds == nil {
		t.Fatalf("full workflow metrics wrong: %+v", complete)
	}
	phase5Float(t, complete.Summary.SelfServiceRate, .5, "completed attempts denominator")
	var trail audit.Page
	decodeTicketJSON(t, f.call(t, f.admin, "GET", "/admin/audit?from="+date+"&to="+date+"&entity_type=ticket&entity_id="+issue.ID+"&limit=100", nil, 200), &trail)
	if trail.Total < 5 {
		t.Fatalf("full workflow audit trail incomplete: %+v", trail)
	}
	// Concurrent administrator settings updates with the same version have one winner.
	var beforeAudits int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='settings.updated'`, f.admin.user.ID).Scan(&beforeAudits); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		status int
		err    error
	}
	start := make(chan struct{})
	done := make(chan outcome, 2)
	for _, name := range []string{"Synthetic clinic A", "Synthetic clinic B"} {
		payload, err := json.Marshal(settings.UpdateInput{Version: saved.Settings.Version, ClinicName: &name})
		if err != nil {
			t.Fatal(err)
		}
		go func(body []byte) {
			<-start
			_, status, err := f.request(f.admin, "PATCH", "/settings", body, "application/json", nil)
			done <- outcome{status, err}
		}(payload)
	}
	close(start)
	statuses := map[int]int{}
	for range 2 {
		select {
		case o := <-done:
			if o.err != nil {
				t.Fatal(o.err)
			}
			statuses[o.status]++
		case <-f.ctx.Done():
			t.Fatal("settings concurrent writes exceeded fixture deadline")
		}
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("settings same-version writes lacked one winner: %v", statuses)
	}
	var final struct {
		Settings settings.Settings `json:"settings"`
	}
	decodeTicketJSON(t, f.call(t, f.admin, "GET", "/settings", nil, 200), &final)
	var afterAudits int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='settings.updated'`, f.admin.user.ID).Scan(&afterAudits); err != nil {
		t.Fatal(err)
	}
	if final.Settings.Version != saved.Settings.Version+1 || afterAudits != beforeAudits+1 {
		t.Fatalf("losing settings write left partial audit/version: version%d expected%d audits%d/%d", final.Settings.Version, saved.Settings.Version+1, afterAudits, beforeAudits)
	}
	// Session still exists, but changed DB permissions deny the reporting routes.
	if _, err := f.pool.Exec(f.ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='EMPLOYEE')WHERE id=$1::uuid`, f.engineer.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call(t, f.engineer, "GET", reportPath, nil, 403)
	f.call(t, f.engineer, "GET", "/analytics/filters", nil, 403)
}
