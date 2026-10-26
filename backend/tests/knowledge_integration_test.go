package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
)

func knowledgeInput(f fixture, slug string, published bool) knowledge.ArticleInput {
	return knowledge.ArticleInput{Title: "NCALayer бағдарламасын іске қосу", Slug: slug, Description: "Принтеры не печатают. Безопасная инструкция.", CategoryID: f.category, Symptoms: "ЭЦҚ қосылмайды. Очередь принтера.", SolutionSteps: []string{"NCALayer ашыңыз", "Перезапустите приложение"}, Tags: []string{"NCALayer", "ЭЦҚ"}, ImageIDs: []string{}, IsPublished: boolValue(published)}
}
func knowledgeUUID(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	queryID(t, ctx, tx, &id, `SELECT gen_random_uuid()::text`)
	return id
}
func TestKnowledgeCRUDMultilingualSearchAndVisibility(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := knowledge.NewService(knowledge.NewRepository(tx))
	slug := "search-" + knowledgeUUID(t, ctx, tx)
	input := knowledgeInput(f, slug, true)
	input.Tags = append(input.Tags, "Қауіпсіздік")
	article, err := service.Save(ctx, actor, "", input)
	if err != nil {
		t.Fatal(err)
	}
	if article.Version != 1 || !article.IsPublished || article.PublishedAt == nil || len(article.Tags) != 3 || !article.CategoryActive {
		t.Fatalf("bad article DTO: %+v", article)
	}
	employee := actor
	employee.Role = auth.RoleEmployee
	for _, search := range []string{"NCALayer", "ЭЦҚ", "принтер", "принтеры", "бағдарлама", "Қауіпсіздік"} {
		page, err := service.List(ctx, employee, knowledge.Filter{Search: search, CategoryID: f.category, Published: boolValue(true), Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range page.Items {
			if item.ID == article.ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("multilingual search %q missed article: %+v", search, page)
		}
	}
	page, err := service.List(ctx, employee, knowledge.Filter{CategoryID: f.category, Tag: "эцқ", Published: boolValue(true), Limit: 1})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("filter/pagination: %+v %v", page, err)
	}
	draftInput := knowledgeInput(f, "draft-"+knowledgeUUID(t, ctx, tx), false)
	draftInput.Tags = []string{"private-tag"}
	draft, err := service.Save(ctx, actor, "", draftInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, employee, draft.ID); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("employee draft leak: %v", err)
	}
	if _, err := service.GetBySlug(ctx, employee, draft.Slug); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("slug draft leak: %v", err)
	}
	if _, err := service.List(ctx, employee, knowledge.Filter{Published: boolValue(false), Limit: 20}); !errors.Is(err, knowledge.ErrForbidden) {
		t.Fatalf("employee draft filter: %v", err)
	}
	tags, err := service.Tags(ctx, employee, "private-tag", 20)
	if err != nil || len(tags) != 0 {
		t.Fatalf("draft tags leaked: %v %v", tags, err)
	}
	input.Version = article.Version
	input.Title = "NCALayer updated"
	article, err = service.Save(ctx, actor, article.ID, input)
	if err != nil || article.Version != 2 {
		t.Fatalf("version update: %+v %v", article, err)
	}
	if _, err := service.Save(ctx, actor, article.ID, input); !errors.Is(err, knowledge.ErrVersionConflict) {
		t.Fatalf("lost update allowed: %v", err)
	}
	if _, err := service.Save(ctx, actor, "", knowledgeInput(f, slug, true)); !errors.Is(err, knowledge.ErrConflict) {
		t.Fatalf("duplicate slug: %v", err)
	}
	category, err := service.SaveCategory(ctx, actor, f.category, knowledge.CategoryInput{Slug: "category-" + knowledgeUUID(t, ctx, tx), NameKK: "Санат", NameRU: "Категория", Description: "Демо", IsActive: boolValue(false)})
	if err != nil || category.IsActive {
		t.Fatalf("category deactivation: %+v %v", category, err)
	}
	if _, err := service.Get(ctx, employee, article.ID); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("inactive category leaked: %v", err)
	}
	article, err = service.Get(ctx, actor, article.ID)
	if err != nil || article.CategoryActive {
		t.Fatalf("staff inactive preview: %+v %v", article, err)
	}
	input.Version = article.Version
	input.IsPublished = boolValue(false)
	article, err = service.Save(ctx, actor, article.ID, input)
	if err != nil || article.IsPublished {
		t.Fatalf("inactive unpublish failed: %+v %v", article, err)
	}
	input.Version = article.Version
	input.IsPublished = boolValue(true)
	if _, err := service.Save(ctx, actor, article.ID, input); !errors.Is(err, knowledge.ErrInactiveCategory) {
		t.Fatalf("inactive publish allowed: %v", err)
	}
	if err := service.Delete(ctx, actor, article.ID, article.Version-1); !errors.Is(err, knowledge.ErrVersionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	execSQL(t, ctx, tx, `INSERT INTO ticket_articles(ticket_id,article_id)VALUES($1,$2)`, f.ticket, article.ID)
	if err := service.Delete(ctx, actor, article.ID, article.Version); !errors.Is(err, knowledge.ErrArticleLinked) {
		t.Fatalf("linked delete: %v", err)
	}
	execSQL(t, ctx, tx, `DELETE FROM ticket_articles WHERE article_id=$1`, article.ID)
	if err := service.Delete(ctx, actor, article.ID, article.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, actor, article.ID); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("deleted article remains: %v", err)
	}
}
func TestKnowledgeViewFeedbackDedupAndFreshAuthorization(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := knowledge.NewService(knowledge.NewRepository(tx))
	article, err := service.Save(ctx, actor, "", knowledgeInput(f, "feedback-"+knowledgeUUID(t, ctx, tx), true))
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.RecordView(ctx, actor, article.ID)
	if err != nil || !first.Recorded || first.ViewCount != 1 {
		t.Fatalf("first view: %+v %v", first, err)
	}
	second, err := service.RecordView(ctx, actor, article.ID)
	if err != nil || second.Recorded || second.ViewCount != 1 {
		t.Fatalf("repeat view: %+v %v", second, err)
	}
	execSQL(t, ctx, tx, `UPDATE article_views SET created_at=clock_timestamp()-interval '16 minutes' WHERE article_id=$1`, article.ID)
	third, err := service.RecordView(ctx, actor, article.ID)
	if err != nil || !third.Recorded || third.ViewCount != 2 {
		t.Fatalf("later view: %+v %v", third, err)
	}
	attempt := knowledgeUUID(t, ctx, tx)
	feedback := knowledge.FeedbackInput{AttemptID: attempt, Helpful: boolValue(true)}
	firstFeedback, err := service.Feedback(ctx, actor, article.ID, feedback)
	if err != nil || !firstFeedback.Recorded || firstFeedback.HelpfulCount != 1 {
		t.Fatalf("feedback: %+v %v", firstFeedback, err)
	}
	repeat, err := service.Feedback(ctx, actor, article.ID, feedback)
	if err != nil || repeat.Recorded || repeat.HelpfulCount != 1 {
		t.Fatalf("repeat feedback: %+v %v", repeat, err)
	}
	feedback.Helpful = boolValue(false)
	if _, err := service.Feedback(ctx, actor, article.ID, feedback); !errors.Is(err, knowledge.ErrFeedbackConflict) {
		t.Fatalf("changed same-attempt: %v", err)
	}
	feedback.AttemptID = knowledgeUUID(t, ctx, tx)
	negative, err := service.Feedback(ctx, actor, article.ID, feedback)
	if err != nil || negative.NotHelpfulCount != 1 || negative.HelpfulCount != 1 {
		t.Fatalf("negative: %+v %v", negative, err)
	}
	draft, err := service.Save(ctx, actor, "", knowledgeInput(f, "hidden-"+knowledgeUUID(t, ctx, tx), false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordView(ctx, actor, draft.ID); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("draft view metric accepted: %v", err)
	}
	if _, err := service.Feedback(ctx, actor, draft.ID, feedback); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("draft feedback metric accepted: %v", err)
	}
	execSQL(t, ctx, tx, `UPDATE users SET is_active=false WHERE id=$1`, actor.ID)
	if _, err := service.RecordView(ctx, actor, article.ID); !errors.Is(err, knowledge.ErrForbidden) {
		t.Fatalf("stale active role accepted: %v", err)
	}
}
func TestKnowledgeMediaOwnershipPurposeAndAuditRollback(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='IT_ENGINEER') WHERE id=$1`, actor.ID)
	actor.Role = auth.RoleEngineer
	service := knowledge.NewService(knowledge.NewRepository(tx))
	other := createFixture(t, ctx, tx)
	ownMedia := knowledgeMediaFixture(t, ctx, tx, actor.ID, "ARTICLE_IMAGE")
	foreignMedia := knowledgeMediaFixture(t, ctx, tx, other.user, "ARTICLE_IMAGE")
	video := knowledgeMediaFixture(t, ctx, tx, actor.ID, "ARTICLE_VIDEO")
	input := knowledgeInput(f, "media-"+knowledgeUUID(t, ctx, tx), false)
	input.ImageIDs = []string{foreignMedia}
	if _, err := service.Save(ctx, actor, "", input); !errors.Is(err, knowledge.ErrForbidden) {
		t.Fatalf("foreign attachment accepted: %v", err)
	}
	input.ImageIDs = []string{video}
	if _, err := service.Save(ctx, actor, "", input); !errors.Is(err, knowledge.ErrInvalidInput) {
		t.Fatalf("wrong media purpose accepted: %v", err)
	}
	input.ImageIDs = []string{ownMedia}
	input.VideoID = &video
	article, err := service.Save(ctx, actor, "", input)
	if err != nil || len(article.ImageIDs) != 1 || article.VideoID == nil {
		t.Fatalf("media association: %+v %v", article, err)
	}
	// Existing associations can be retained by another engineer editing the article.
	execSQL(t, ctx, tx, `UPDATE media_files SET uploader_id=$2 WHERE id=$1`, ownMedia, other.user)
	input.Version = article.Version
	input.Title = "Attached media retained"
	article, err = service.Save(ctx, actor, article.ID, input)
	if err != nil {
		t.Fatalf("existing attached media rejected: %v", err)
	}
	execSQL(t, ctx, tx, `CREATE FUNCTION pg_temp.reject_knowledge_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='article.updated' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_knowledge_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_knowledge_audit()`)
	input.Version = article.Version
	input.Title = "Must roll back"
	input.Tags = []string{"must-rollback"}
	if _, err := service.Save(ctx, actor, article.ID, input); err == nil {
		t.Fatal("audit failure unexpectedly succeeded")
	}
	stored, err := service.Get(ctx, actor, article.ID)
	if err != nil || stored.Title != article.Title || stored.Version != article.Version {
		t.Fatalf("audit failure did not roll back: %+v %v", stored, err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_tags WHERE name='must-rollback'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("association rollback failed: %d %v", count, err)
	}
	employee := actor
	employee.Role = auth.RoleEmployee
	if _, err := service.Save(ctx, employee, "", knowledgeInput(f, "employee", true)); !errors.Is(err, knowledge.ErrForbidden) {
		t.Fatalf("employee authored article: %v", err)
	}
}
func knowledgeMediaFixture(t *testing.T, ctx context.Context, tx pgx.Tx, user, purpose string) string {
	t.Helper()
	ext, mime := "png", "image/png"
	if purpose == "ARTICLE_VIDEO" {
		ext, mime = "mp4", "video/mp4"
	}
	var id string
	queryID(t, ctx, tx, &id, `INSERT INTO media_files(uploader_id,storage_key,original_filename,content_type,byte_count,sha256,purpose) VALUES($1,replace(gen_random_uuid()::text,'-','')||'.'||$2,'synthetic.'||$2,$3,100,$4,$5)RETURNING id::text`, user, ext, mime, []byte(strings.Repeat("a", 32)), purpose)
	return id
}

func TestKnowledgeCategoryManagementAndLocalizedFiltering(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, _ := managementAdmin(t, ctx, tx)
	service := knowledge.NewService(knowledge.NewRepository(tx))
	marker := knowledgeUUID(t, ctx, tx)
	input := knowledge.CategoryInput{Slug: "category-" + marker, NameKK: "Құрылғы " + marker, NameRU: "Устройство " + marker, Description: "Synthetic category", IsActive: boolValue(true)}
	category, err := service.SaveCategory(ctx, actor, "", input)
	if err != nil || !category.IsActive {
		t.Fatalf("create category: %+v %v", category, err)
	}
	if _, err := service.SaveCategory(ctx, actor, "", input); !errors.Is(err, knowledge.ErrConflict) {
		t.Fatalf("duplicate category slug: %v", err)
	}
	employee := actor
	employee.Role = auth.RoleEmployee
	for _, search := range []string{"Құрылғы " + marker, "Устройство " + marker} {
		page, err := service.Categories(ctx, employee, knowledge.CategoryFilter{Search: search, Limit: 20})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != category.ID {
			t.Fatalf("localized category filter %q: %+v %v", search, page, err)
		}
	}
	engineer := actor
	engineer.Role = auth.RoleEngineer
	if _, err := service.SaveCategory(ctx, engineer, category.ID, input); !errors.Is(err, knowledge.ErrForbidden) {
		t.Fatalf("engineer category update: %v", err)
	}
	input.IsActive = boolValue(false)
	category, err = service.SaveCategory(ctx, actor, category.ID, input)
	if err != nil || category.IsActive {
		t.Fatalf("deactivate category: %+v %v", category, err)
	}
	page, err := service.Categories(ctx, employee, knowledge.CategoryFilter{Search: marker, Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("inactive categories visible: %+v %v", page, err)
	}
	if _, err := service.Categories(ctx, employee, knowledge.CategoryFilter{Active: boolValue(false), Limit: 20}); !errors.Is(err, knowledge.ErrForbidden) {
		t.Fatalf("employee inactive filter: %v", err)
	}
	page, err = service.Categories(ctx, actor, knowledge.CategoryFilter{Search: marker, Active: boolValue(false), Limit: 20})
	if err != nil || page.Total != 1 {
		t.Fatalf("admin inactive filter: %+v %v", page, err)
	}
}

func TestKnowledgeRussianTranslation(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	actor, f := managementAdmin(t, ctx, tx)
	service := knowledge.NewService(knowledge.NewRepository(tx))
	input := knowledgeInput(f, "translation-"+knowledgeUUID(t, ctx, tx), true)
	input.TranslationRU = &knowledge.ArticleTranslation{Title: "Диагностика видеоконференции", Description: "Проверка оборудования", Symptoms: "Микрофон недоступен", SolutionSteps: []string{"Проверьте подключение"}}
	article, err := service.Save(ctx, actor, "", input)
	if err != nil {
		t.Fatal(err)
	}
	if article.TranslationRU == nil || article.TranslationRU.Title != input.TranslationRU.Title {
		t.Fatalf("translation not persisted: %+v", article)
	}
	page, err := service.List(ctx, actor, knowledge.Filter{Search: "видеоконференции", CategoryID: f.category, Limit: 20})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != article.ID {
		t.Fatalf("translated search failed: %+v %v", page, err)
	}
	input.Version = article.Version
	input.TranslationRU = nil
	article, err = service.Save(ctx, actor, article.ID, input)
	if err != nil || article.TranslationRU == nil {
		t.Fatalf("legacy update erased translation: %+v %v", article, err)
	}
}
