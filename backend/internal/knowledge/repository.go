package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

type Database interface {
	db.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct {
	database Database
	q        *db.Queries
}

func NewRepository(database Database) *Repository {
	return &Repository{database: database, q: db.New(database)}
}
func uid(s string) pgtype.UUID { var id pgtype.UUID; _ = id.Scan(s); return id }
func uuidString(id pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
func boolParam(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
func videoParam(v *string) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return uid(*v)
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrConflict
		case "23514", "22001":
			return ErrInvalidInput
		}
	}
	return err
}
func articleDTO(row db.KnowledgeArticleDetailsRow) (Article, error) {
	steps := []string{}
	if err := json.Unmarshal(row.SolutionSteps, &steps); err != nil {
		return Article{}, err
	}
	images := make([]string, 0, len(row.ImageIds))
	for _, id := range row.ImageIds {
		images = append(images, uuidString(id))
	}
	tags := row.Tags
	if tags == nil {
		tags = []string{}
	}
	article := Article{ID: uuidString(row.ID), Version: row.Version, Title: row.Title, Slug: row.Slug, Description: row.Description, CategoryID: uuidString(row.CategoryID), CategoryNameKK: row.CategoryNameKk, CategoryNameRU: row.CategoryNameRu, CategoryActive: row.CategoryActive, Symptoms: row.Symptoms, SolutionSteps: steps, Tags: tags, AuthorID: uuidString(row.AuthorID), AuthorName: row.AuthorName, ImageIDs: images, IsPublished: row.IsPublished, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, ViewCount: row.ViewCount, HelpfulCount: row.HelpfulCount, NotHelpfulCount: row.NotHelpfulCount}
	if row.VideoID.Valid {
		v := uuidString(row.VideoID)
		article.VideoID = &v
	}
	if row.PublishedAt.Valid {
		v := row.PublishedAt.Time
		article.PublishedAt = &v
	}
	return article, nil
}
func categoryDTO(row db.KnowledgeCategory) Category {
	return Category{ID: uuidString(row.ID), Slug: row.Slug, NameKK: row.NameKk, NameRU: row.NameRu, Description: row.Description, IsActive: row.IsActive, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}
func getArticle(ctx context.Context, q *db.Queries, id pgtype.UUID, public bool) (Article, error) {
	rows, err := q.KnowledgeArticleDetails(ctx, db.KnowledgeArticleDetailsParams{Ids: []pgtype.UUID{id}, PublicOnly: public})
	if err != nil {
		return Article{}, mapError(err)
	}
	if len(rows) != 1 {
		return Article{}, ErrNotFound
	}
	return articleDTO(rows[0])
}
func (r *Repository) GetByID(ctx context.Context, id string, public bool) (Article, error) {
	return getArticle(ctx, r.q, uid(id), public)
}
func (r *Repository) GetBySlug(ctx context.Context, slug string, public bool) (Article, error) {
	id, err := r.q.KnowledgeFindSlug(ctx, db.KnowledgeFindSlugParams{Slug: slug, PublicOnly: public})
	if err != nil {
		return Article{}, mapError(err)
	}
	return getArticle(ctx, r.q, id, public)
}
func (r *Repository) List(ctx context.Context, f Filter) (ArticlePage, error) {
	published := boolParam(f.Published)
	category := uid(f.CategoryID)
	ids, err := r.q.KnowledgeListArticleIDs(ctx, db.KnowledgeListArticleIDsParams{PublicOnly: f.PublicOnly, Published: published, CategoryID: category, Tag: f.Tag, Search: f.Search, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset)})
	if err != nil {
		return ArticlePage{}, err
	}
	total, err := r.q.KnowledgeCountArticles(ctx, db.KnowledgeCountArticlesParams{PublicOnly: f.PublicOnly, Published: published, CategoryID: category, Tag: f.Tag, Search: f.Search})
	if err != nil {
		return ArticlePage{}, err
	}
	rows, err := r.q.KnowledgeArticleDetails(ctx, db.KnowledgeArticleDetailsParams{Ids: ids, PublicOnly: f.PublicOnly})
	if err != nil {
		return ArticlePage{}, err
	}
	items := make([]Article, 0, len(rows))
	for _, row := range rows {
		article, err := articleDTO(row)
		if err != nil {
			return ArticlePage{}, err
		}
		items = append(items, article)
	}
	return ArticlePage{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}
func (r *Repository) Categories(ctx context.Context, f CategoryFilter) (CategoryPage, error) {
	active := boolParam(f.Active)
	rows, err := r.q.KnowledgeListCategories(ctx, db.KnowledgeListCategoriesParams{PublicOnly: f.PublicOnly, Active: active, Search: f.Search, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset)})
	if err != nil {
		return CategoryPage{}, err
	}
	total, err := r.q.KnowledgeCountCategories(ctx, db.KnowledgeCountCategoriesParams{PublicOnly: f.PublicOnly, Active: active, Search: f.Search})
	if err != nil {
		return CategoryPage{}, err
	}
	items := make([]Category, 0, len(rows))
	for _, row := range rows {
		items = append(items, categoryDTO(row))
	}
	return CategoryPage{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}
func (r *Repository) Tags(ctx context.Context, search string, limit int, public bool) ([]string, error) {
	return r.q.KnowledgeTags(ctx, db.KnowledgeTagsParams{Search: search, PageLimit: int32(limit), PublicOnly: public})
}
func (r *Repository) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(&mutationRepository{q: db.New(tx)}); err != nil {
		return mapError(err)
	}
	return mapError(tx.Commit(ctx))
}

type mutationRepository struct{ q *db.Queries }

func (r *mutationRepository) Lock(ctx context.Context) error { return r.q.LockUserManagement(ctx) }
func (r *mutationRepository) Actor(ctx context.Context, id string) (auth.User, error) {
	u, err := r.q.GetManagedUserForUpdate(ctx, uid(id))
	if err != nil {
		return auth.User{}, mapError(err)
	}
	return auth.User{ID: uuidString(u.ID), Email: u.Email, DisplayName: u.DisplayName, Role: u.Role, DepartmentID: uuidString(u.DepartmentID), DepartmentName: u.DepartmentName, IsActive: u.IsActive}, nil
}
func (r *mutationRepository) Category(ctx context.Context, id string) (Category, error) {
	row, err := r.q.KnowledgeGetCategory(ctx, uid(id))
	return categoryDTO(row), mapError(err)
}
func (r *mutationRepository) Article(ctx context.Context, id string, public bool) (Article, error) {
	if _, err := r.q.KnowledgeLockArticle(ctx, uid(id)); err != nil {
		return Article{}, mapError(err)
	}
	return getArticle(ctx, r.q, uid(id), public)
}
func (r *mutationRepository) CreateArticle(ctx context.Context, author string, input ArticleInput) (Article, error) {
	steps, err := json.Marshal(input.SolutionSteps)
	if err != nil {
		return Article{}, err
	}
	id, err := r.q.KnowledgeCreateArticle(ctx, db.KnowledgeCreateArticleParams{Title: input.Title, Slug: input.Slug, Description: input.Description, CategoryID: uid(input.CategoryID), Symptoms: input.Symptoms, SolutionSteps: steps, AuthorID: uid(author), VideoID: videoParam(input.VideoID), IsPublished: *input.IsPublished})
	if err != nil {
		return Article{}, mapError(err)
	}
	if err := r.associations(ctx, id, input); err != nil {
		return Article{}, err
	}
	return getArticle(ctx, r.q, id, false)
}
func (r *mutationRepository) UpdateArticle(ctx context.Context, id string, input ArticleInput) (Article, error) {
	steps, err := json.Marshal(input.SolutionSteps)
	if err != nil {
		return Article{}, err
	}
	err = r.q.KnowledgeUpdateArticle(ctx, db.KnowledgeUpdateArticleParams{ID: uid(id), Title: input.Title, Slug: input.Slug, Description: input.Description, CategoryID: uid(input.CategoryID), Symptoms: input.Symptoms, SolutionSteps: steps, VideoID: videoParam(input.VideoID), IsPublished: *input.IsPublished})
	if err != nil {
		return Article{}, mapError(err)
	}
	if err := r.associations(ctx, uid(id), input); err != nil {
		return Article{}, err
	}
	return getArticle(ctx, r.q, uid(id), false)
}
func (r *mutationRepository) associations(ctx context.Context, id pgtype.UUID, input ArticleInput) error {
	if err := r.q.KnowledgeRemoveArticleTags(ctx, id); err != nil {
		return err
	}
	for _, tag := range input.Tags {
		tagID, err := r.q.KnowledgeEnsureTag(ctx, tag)
		if err != nil {
			return err
		}
		if err := r.q.KnowledgeLinkTag(ctx, db.KnowledgeLinkTagParams{ArticleID: id, TagID: tagID}); err != nil {
			return err
		}
	}
	if err := r.q.KnowledgeRemoveImages(ctx, id); err != nil {
		return err
	}
	for i, mediaID := range input.ImageIDs {
		if err := r.q.KnowledgeLinkImage(ctx, db.KnowledgeLinkImageParams{ArticleID: id, MediaID: uid(mediaID), Position: int32(i)}); err != nil {
			return err
		}
	}
	return nil
}
func (r *mutationRepository) DeleteArticle(ctx context.Context, id string) error {
	return r.q.KnowledgeDeleteArticle(ctx, uid(id))
}
func (r *mutationRepository) Linked(ctx context.Context, id string) (bool, error) {
	return r.q.KnowledgeArticleTicketLinks(ctx, uid(id))
}
func (r *mutationRepository) CheckMedia(ctx context.Context, mediaID, purpose, actorID, articleID string, admin bool) error {
	media, err := r.q.KnowledgeMediaForUpdate(ctx, uid(mediaID))
	if err != nil {
		return mapError(err)
	}
	if media.Purpose != purpose {
		return ErrInvalidInput
	}
	if admin || uuidString(media.UploaderID) == actorID {
		return nil
	}
	if articleID != "" {
		attached, err := r.q.KnowledgeMediaAttached(ctx, db.KnowledgeMediaAttachedParams{ID: uid(articleID), VideoID: uid(mediaID)})
		if err != nil {
			return err
		}
		if attached.Valid && attached.Bool {
			return nil
		}
	}
	return ErrForbidden
}
func (r *mutationRepository) CreateCategory(ctx context.Context, input CategoryInput) (Category, error) {
	row, err := r.q.KnowledgeCreateCategory(ctx, db.KnowledgeCreateCategoryParams{NameKk: input.NameKK, NameRu: input.NameRU, Slug: input.Slug, Description: input.Description, IsActive: *input.IsActive})
	return categoryDTO(row), mapError(err)
}
func (r *mutationRepository) UpdateCategory(ctx context.Context, id string, input CategoryInput) (Category, error) {
	row, err := r.q.KnowledgeUpdateCategory(ctx, db.KnowledgeUpdateCategoryParams{ID: uid(id), NameKk: input.NameKK, NameRu: input.NameRU, Slug: input.Slug, Description: input.Description, IsActive: *input.IsActive})
	return categoryDTO(row), mapError(err)
}
func (r *mutationRepository) View(ctx context.Context, articleID, actorID string) (bool, error) {
	_, err := r.q.KnowledgeRecordView(ctx, db.KnowledgeRecordViewParams{ArticleID: uid(articleID), UserID: uid(actorID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
func (r *mutationRepository) Feedback(ctx context.Context, articleID, actorID, attemptID string, helpful bool) (bool, error) {
	value, err := r.q.KnowledgeFeedback(ctx, db.KnowledgeFeedbackParams{ArticleID: uid(articleID), UserID: uid(actorID), AttemptID: uid(attemptID)})
	if err == nil {
		if value != helpful {
			return false, ErrFeedbackConflict
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	err = r.q.KnowledgeInsertFeedback(ctx, db.KnowledgeInsertFeedbackParams{ArticleID: uid(articleID), UserID: uid(actorID), AttemptID: uid(attemptID), Helpful: helpful})
	return err == nil, err
}
func (r *mutationRepository) Audit(ctx context.Context, actor, action, entity, id string, metadata map[string]any) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return r.q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{ActorID: uid(actor), Action: action, EntityType: entity, EntityID: uid(id), Metadata: payload})
}
