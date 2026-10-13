package knowledge

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
)

type Store interface {
	List(context.Context, Filter) (ArticlePage, error)
	GetBySlug(context.Context, string, bool) (Article, error)
	GetByID(context.Context, string, bool) (Article, error)
	Categories(context.Context, CategoryFilter) (CategoryPage, error)
	Tags(context.Context, string, int, bool) ([]string, error)
	Transaction(context.Context, func(MutationStore) error) error
}
type MutationStore interface {
	Lock(context.Context) error
	Actor(context.Context, string) (auth.User, error)
	Category(context.Context, string) (Category, error)
	Article(context.Context, string, bool) (Article, error)
	CreateArticle(context.Context, string, ArticleInput) (Article, error)
	UpdateArticle(context.Context, string, ArticleInput) (Article, error)
	DeleteArticle(context.Context, string) error
	Linked(context.Context, string) (bool, error)
	CheckMedia(context.Context, string, string, string, string, bool) error
	CreateCategory(context.Context, CategoryInput) (Category, error)
	UpdateCategory(context.Context, string, CategoryInput) (Category, error)
	View(context.Context, string, string) (bool, error)
	Feedback(context.Context, string, string, string, bool) (bool, error)
	Audit(context.Context, string, string, string, string, map[string]any) error
}
type Service struct{ repo Store }

func NewService(repo Store) *Service { return &Service{repo: repo} }
func staff(actor auth.User) bool {
	return actor.IsActive && (actor.Role == auth.RoleAdmin || actor.Role == auth.RoleEngineer)
}
func active(actor auth.User) bool {
	return actor.IsActive && (actor.Role == auth.RoleAdmin || actor.Role == auth.RoleEngineer || actor.Role == auth.RoleEmployee)
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func validSlug(s string, max int) bool { return len(s) <= max && slugPattern.MatchString(s) }
func validBody(s string, max int, empty bool) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max || (!empty && strings.TrimSpace(s) == "") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func validPage(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 100000
}
func (s *Service) List(ctx context.Context, actor auth.User, f Filter) (ArticlePage, error) {
	if !active(actor) {
		return ArticlePage{}, ErrForbidden
	}
	f.Search = strings.TrimSpace(f.Search)
	f.Tag = strings.TrimSpace(f.Tag)
	if !validPage(f.Limit, f.Offset) || (f.Search != "" && !httpx.ValidText(f.Search, 100)) || (f.Tag != "" && !httpx.ValidText(f.Tag, 60)) || (f.CategoryID != "" && !httpx.ValidUUID(f.CategoryID)) {
		return ArticlePage{}, ErrInvalidInput
	}
	if !staff(actor) && f.Published != nil && !*f.Published {
		return ArticlePage{}, ErrForbidden
	}
	// Every employee query is constrained independently of client filters.
	if !staff(actor) {
		f.PublicOnly = true
		yes := true
		f.Published = &yes
	}
	return s.repo.List(ctx, f)
}
func (s *Service) Get(ctx context.Context, actor auth.User, id string) (Article, error) {
	if !active(actor) {
		return Article{}, ErrForbidden
	}
	if !httpx.ValidUUID(id) {
		return Article{}, ErrInvalidInput
	}
	return s.repo.GetByID(ctx, id, !staff(actor))
}
func (s *Service) GetBySlug(ctx context.Context, actor auth.User, slug string) (Article, error) {
	if !active(actor) {
		return Article{}, ErrForbidden
	}
	if !validSlug(slug, 200) {
		return Article{}, ErrInvalidInput
	}
	return s.repo.GetBySlug(ctx, slug, !staff(actor))
}
func normalizeArticle(input ArticleInput) (ArticleInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Description = strings.TrimSpace(input.Description)
	input.Symptoms = strings.TrimSpace(input.Symptoms)
	input.CategoryID = strings.ToLower(input.CategoryID)
	if !httpx.ValidText(input.Title, 250) || !validSlug(input.Slug, 200) || !validBody(input.Description, 2000, false) || !validBody(input.Symptoms, 10000, false) || !httpx.ValidUUID(input.CategoryID) || input.IsPublished == nil || len(input.SolutionSteps) < 1 || len(input.SolutionSteps) > 100 || len(input.Tags) > 10 || len(input.ImageIDs) > 10 {
		return input, ErrInvalidInput
	}
	for i, step := range input.SolutionSteps {
		step = strings.TrimSpace(step)
		if !validBody(step, 5000, false) {
			return input, ErrInvalidInput
		}
		input.SolutionSteps[i] = step
	}
	tags := make([]string, 0, len(input.Tags))
	seen := map[string]bool{}
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if !httpx.ValidText(tag, 60) {
			return input, ErrInvalidInput
		}
		key := strings.ToLower(tag)
		if !seen[key] {
			seen[key] = true
			tags = append(tags, tag)
		}
	}
	input.Tags = tags
	seen = map[string]bool{}
	for i, id := range input.ImageIDs {
		id = strings.ToLower(id)
		if !httpx.ValidUUID(id) || seen[id] {
			return input, ErrInvalidInput
		}
		seen[id] = true
		input.ImageIDs[i] = id
	}
	if input.VideoID != nil {
		id := strings.ToLower(*input.VideoID)
		if !httpx.ValidUUID(id) {
			return input, ErrInvalidInput
		}
		input.VideoID = &id
	}
	return input, nil
}
func authorize(ctx context.Context, tx MutationStore, actor auth.User, adminOnly bool) (auth.User, error) {
	if err := tx.Lock(ctx); err != nil {
		return auth.User{}, err
	}
	current, err := tx.Actor(ctx, actor.ID)
	if err != nil {
		return auth.User{}, err
	}
	if !active(current) || (adminOnly && current.Role != auth.RoleAdmin) {
		return auth.User{}, ErrForbidden
	}
	return current, nil
}
func (s *Service) Save(ctx context.Context, actor auth.User, id string, input ArticleInput) (Article, error) {
	if !staff(actor) {
		return Article{}, ErrForbidden
	}
	if id != "" && (!httpx.ValidUUID(id) || input.Version < 1) {
		return Article{}, ErrInvalidInput
	}
	if id == "" && input.Version != 0 {
		return Article{}, ErrInvalidInput
	}
	normalized, err := normalizeArticle(input)
	if err != nil {
		return Article{}, err
	}
	id = strings.ToLower(id)
	var article Article
	err = s.repo.Transaction(ctx, func(tx MutationStore) error {
		current, err := authorize(ctx, tx, actor, false)
		if err != nil {
			return err
		}
		if !staff(current) {
			return ErrForbidden
		}
		category, err := tx.Category(ctx, normalized.CategoryID)
		if err != nil {
			return err
		}
		if !category.IsActive && *normalized.IsPublished {
			return ErrInactiveCategory
		}
		if id != "" {
			existing, err := tx.Article(ctx, id, false)
			if err != nil {
				return err
			}
			if existing.Version != normalized.Version {
				return ErrVersionConflict
			}
			if existing.Version == 2147483647 {
				return ErrVersionConflict
			}
		}
		if normalized.VideoID != nil {
			if err := tx.CheckMedia(ctx, *normalized.VideoID, "ARTICLE_VIDEO", current.ID, id, current.Role == auth.RoleAdmin); err != nil {
				return err
			}
		}
		for _, mediaID := range normalized.ImageIDs {
			if err := tx.CheckMedia(ctx, mediaID, "ARTICLE_IMAGE", current.ID, id, current.Role == auth.RoleAdmin); err != nil {
				return err
			}
		}
		action := "article.created"
		if id == "" {
			article, err = tx.CreateArticle(ctx, current.ID, normalized)
		} else {
			action = "article.updated"
			article, err = tx.UpdateArticle(ctx, id, normalized)
		}
		if err != nil {
			return err
		}
		return tx.Audit(ctx, current.ID, action, "knowledge_article", article.ID, map[string]any{"version": article.Version, "is_published": article.IsPublished, "category_id": article.CategoryID})
	})
	return article, err
}
func (s *Service) Delete(ctx context.Context, actor auth.User, id string, version int32) error {
	if !staff(actor) {
		return ErrForbidden
	}
	if !httpx.ValidUUID(id) || version < 1 {
		return ErrInvalidInput
	}
	return s.repo.Transaction(ctx, func(tx MutationStore) error {
		current, err := authorize(ctx, tx, actor, false)
		if err != nil {
			return err
		}
		if !staff(current) {
			return ErrForbidden
		}
		article, err := tx.Article(ctx, id, false)
		if err != nil {
			return err
		}
		if article.Version != version {
			return ErrVersionConflict
		}
		linked, err := tx.Linked(ctx, id)
		if err != nil {
			return err
		}
		if linked {
			return ErrArticleLinked
		}
		if err := tx.DeleteArticle(ctx, id); err != nil {
			return err
		}
		return tx.Audit(ctx, current.ID, "article.deleted", "knowledge_article", id, map[string]any{"version": version})
	})
}
func (s *Service) Categories(ctx context.Context, actor auth.User, f CategoryFilter) (CategoryPage, error) {
	if !active(actor) {
		return CategoryPage{}, ErrForbidden
	}
	f.Search = strings.TrimSpace(f.Search)
	if !validPage(f.Limit, f.Offset) || (f.Search != "" && !httpx.ValidText(f.Search, 100)) {
		return CategoryPage{}, ErrInvalidInput
	}
	if !staff(actor) {
		f.PublicOnly = true
		if f.Active != nil && !*f.Active {
			return CategoryPage{}, ErrForbidden
		}
	}
	return s.repo.Categories(ctx, f)
}
func normalizeCategory(input CategoryInput) (CategoryInput, error) {
	input.Slug = strings.TrimSpace(input.Slug)
	input.NameKK = strings.TrimSpace(input.NameKK)
	input.NameRU = strings.TrimSpace(input.NameRU)
	input.Description = strings.TrimSpace(input.Description)
	if !validSlug(input.Slug, 100) || !httpx.ValidText(input.NameKK, 150) || !httpx.ValidText(input.NameRU, 150) || !validBody(input.Description, 2000, true) || input.IsActive == nil {
		return input, ErrInvalidInput
	}
	return input, nil
}
func (s *Service) SaveCategory(ctx context.Context, actor auth.User, id string, input CategoryInput) (Category, error) {
	if !actor.IsActive || actor.Role != auth.RoleAdmin {
		return Category{}, ErrForbidden
	}
	if id != "" && !httpx.ValidUUID(id) {
		return Category{}, ErrInvalidInput
	}
	normalized, err := normalizeCategory(input)
	if err != nil {
		return Category{}, err
	}
	var category Category
	err = s.repo.Transaction(ctx, func(tx MutationStore) error {
		current, err := authorize(ctx, tx, actor, true)
		if err != nil {
			return err
		}
		action := "category.created"
		if id == "" {
			category, err = tx.CreateCategory(ctx, normalized)
		} else {
			if _, err := tx.Category(ctx, id); err != nil {
				return err
			}
			action = "category.updated"
			category, err = tx.UpdateCategory(ctx, id, normalized)
		}
		if err != nil {
			return err
		}
		return tx.Audit(ctx, current.ID, action, "knowledge_category", category.ID, map[string]any{"is_active": category.IsActive})
	})
	return category, err
}
func (s *Service) Tags(ctx context.Context, actor auth.User, search string, limit int) ([]string, error) {
	if !active(actor) {
		return nil, ErrForbidden
	}
	search = strings.TrimSpace(search)
	if limit < 1 || limit > 100 || (search != "" && !httpx.ValidText(search, 60)) {
		return nil, ErrInvalidInput
	}
	return s.repo.Tags(ctx, search, limit, !staff(actor))
}
func (s *Service) RecordView(ctx context.Context, actor auth.User, id string) (ViewResult, error) {
	if !active(actor) {
		return ViewResult{}, ErrForbidden
	}
	if !httpx.ValidUUID(id) {
		return ViewResult{}, ErrInvalidInput
	}
	var result ViewResult
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		current, err := authorize(ctx, tx, actor, false)
		if err != nil {
			return err
		}
		if _, err := tx.Article(ctx, id, true); err != nil {
			return err
		}
		result.Recorded, err = tx.View(ctx, id, current.ID)
		if err != nil {
			return err
		}
		article, err := tx.Article(ctx, id, true)
		result.ViewCount = article.ViewCount
		return err
	})
	return result, err
}
func (s *Service) Feedback(ctx context.Context, actor auth.User, id string, input FeedbackInput) (FeedbackResult, error) {
	if !active(actor) {
		return FeedbackResult{}, ErrForbidden
	}
	if !httpx.ValidUUID(id) || !httpx.ValidUUID(input.AttemptID) || input.Helpful == nil {
		return FeedbackResult{}, ErrInvalidInput
	}
	var result FeedbackResult
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		current, err := authorize(ctx, tx, actor, false)
		if err != nil {
			return err
		}
		if _, err := tx.Article(ctx, id, true); err != nil {
			return err
		}
		result.Recorded, err = tx.Feedback(ctx, id, current.ID, input.AttemptID, *input.Helpful)
		if err != nil {
			return err
		}
		article, err := tx.Article(ctx, id, true)
		result.HelpfulCount = article.HelpfulCount
		result.NotHelpfulCount = article.NotHelpfulCount
		return err
	})
	return result, err
}
