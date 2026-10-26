// Package knowledge owns troubleshooting content and self-service events.
package knowledge

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput     = errors.New("invalid knowledge input")
	ErrForbidden        = errors.New("knowledge operation forbidden")
	ErrNotFound         = errors.New("knowledge resource not found")
	ErrConflict         = errors.New("knowledge resource conflict")
	ErrVersionConflict  = errors.New("article version conflict")
	ErrInactiveCategory = errors.New("category is inactive")
	ErrArticleLinked    = errors.New("article is linked to a ticket")
	ErrFeedbackConflict = errors.New("attempt feedback is immutable")
)

type Article struct {
	TranslationRU   *ArticleTranslation `json:"translation_ru"`
	CategoryActive  bool                `json:"category_active"`
	ID              string              `json:"id"`
	Version         int32               `json:"version"`
	Title           string              `json:"title"`
	Slug            string              `json:"slug"`
	Description     string              `json:"description"`
	CategoryID      string              `json:"category_id"`
	CategoryNameKK  string              `json:"category_name_kk"`
	CategoryNameRU  string              `json:"category_name_ru"`
	Symptoms        string              `json:"symptoms"`
	SolutionSteps   []string            `json:"solution_steps"`
	Tags            []string            `json:"tags"`
	AuthorID        string              `json:"author_id"`
	AuthorName      string              `json:"author_name"`
	VideoID         *string             `json:"video_id"`
	ImageIDs        []string            `json:"image_ids"`
	IsPublished     bool                `json:"is_published"`
	PublishedAt     *time.Time          `json:"published_at"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	ViewCount       int64               `json:"view_count"`
	HelpfulCount    int64               `json:"helpful_count"`
	NotHelpfulCount int64               `json:"not_helpful_count"`
}

// The original article fields are Kazakh; a complete Russian translation is optional.
type ArticleTranslation struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Symptoms      string   `json:"symptoms"`
	SolutionSteps []string `json:"solution_steps"`
}
type Category struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	NameKK      string    `json:"name_kk"`
	NameRU      string    `json:"name_ru"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type ArticleInput struct {
	TranslationRU *ArticleTranslation `json:"translation_ru,omitempty"`
	Version       int32               `json:"version,omitempty"`
	Title         string              `json:"title"`
	Slug          string              `json:"slug"`
	Description   string              `json:"description"`
	CategoryID    string              `json:"category_id"`
	Symptoms      string              `json:"symptoms"`
	SolutionSteps []string            `json:"solution_steps"`
	Tags          []string            `json:"tags"`
	VideoID       *string             `json:"video_id"`
	ImageIDs      []string            `json:"image_ids"`
	IsPublished   *bool               `json:"is_published"`
}
type CategoryInput struct {
	Slug        string `json:"slug"`
	NameKK      string `json:"name_kk"`
	NameRU      string `json:"name_ru"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}
type Filter struct {
	Search, CategoryID, Tag string
	Published               *bool
	PublicOnly              bool
	Limit, Offset           int
}
type CategoryFilter struct {
	Search        string
	Active        *bool
	PublicOnly    bool
	Limit, Offset int
}
type ArticlePage struct {
	Items  []Article `json:"items"`
	Total  int64     `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}
type CategoryPage struct {
	Items  []Category `json:"items"`
	Total  int64      `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}
type FeedbackInput struct {
	AttemptID string `json:"attempt_id"`
	Helpful   *bool  `json:"helpful"`
}
type FeedbackResult struct {
	Recorded        bool  `json:"recorded"`
	HelpfulCount    int64 `json:"helpful_count"`
	NotHelpfulCount int64 `json:"not_helpful_count"`
}
type ViewResult struct {
	Recorded  bool  `json:"recorded"`
	ViewCount int64 `json:"view_count"`
}
type DeleteInput struct {
	Version int32 `json:"version"`
}
