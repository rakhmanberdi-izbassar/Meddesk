// Package tickets owns durable support workflows and their authorization policy.
package tickets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
)

const (
	New        = "NEW"
	InProgress = "IN_PROGRESS"
	Waiting    = "WAITING_FOR_USER"
	Resolved   = "RESOLVED"
	Closed     = "CLOSED"
)

var (
	ErrInvalidInput     = errors.New("invalid ticket input")
	ErrForbidden        = errors.New("ticket operation forbidden")
	ErrNotFound         = errors.New("ticket not found")
	ErrVersionConflict  = errors.New("ticket version conflict")
	ErrTransition       = errors.New("invalid ticket transition")
	ErrClosed           = errors.New("ticket is closed")
	ErrInactiveCategory = errors.New("category is inactive")
	ErrConflict         = errors.New("ticket resource conflict")
	ErrAlreadyConverted = errors.New("ticket already converted to an article")
	ErrExportLimit      = errors.New("ticket export limit exceeded")
)

type Ticket struct {
	ID                 string     `json:"id"`
	Number             int64      `json:"number"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	RequesterID        string     `json:"requester_id"`
	RequesterName      string     `json:"requester_name"`
	DepartmentID       string     `json:"department_id"`
	DepartmentName     string     `json:"department_name"`
	CategoryID         *string    `json:"category_id"`
	Priority           string     `json:"priority"`
	Status             string     `json:"status"`
	AssigneeID         *string    `json:"assignee_id"`
	AssigneeName       string     `json:"assignee_name"`
	Version            int32      `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ResolvedAt         *time.Time `json:"resolved_at"`
	ArticleIDs         []string   `json:"article_ids"`
	AttachmentIDs      []string   `json:"attachment_ids"`
	ConvertedArticleID *string    `json:"converted_article_id"`
}
type Comment struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	IsInternal bool      `json:"is_internal"`
	CreatedAt  time.Time `json:"created_at"`
}
type History struct {
	ID         int64     `json:"id"`
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	ActorName  string    `json:"actor_name"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}
type Detail struct {
	Ticket   Ticket    `json:"ticket"`
	Comments []Comment `json:"comments"`
	History  []History `json:"history"`
}
type Page struct {
	Items  []Ticket `json:"items"`
	Total  int64    `json:"total"`
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}
type Department struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Assignee struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}
type Filter struct {
	Search, Status, Priority, AssigneeID, DepartmentID, CategoryID string
	From, To, Timezone                                             string
	Mine                                                           bool
	Limit, Offset                                                  int
}

// ExportRow contains only the fields approved for management reports.
type ExportRow struct {
	Number                                                       int64
	Title, RequesterName, DepartmentName, CategoryKK, CategoryRU string
	Priority, Status, AssigneeName                               string
	CreatedAt, UpdatedAt                                         time.Time
	ResolvedAt                                                   *time.Time
}
type ExportSnapshot struct {
	ClinicName, Timezone string
	Period               *analytics.Period
	Rows                 []ExportRow
	FilterLabels         ExportFilterLabels
}
type ExportFilterLabels struct {
	DepartmentName, CategoryKK, CategoryRU, AssigneeName string
}
type ExportInput struct {
	Filter         Filter
	Format, Locale string
}
type ExportFile struct {
	Data                  []byte
	Filename, ContentType string
}
type CreateInput struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Priority      string   `json:"priority"`
	CategoryID    *string  `json:"category_id"`
	ArticleIDs    []string `json:"article_ids"`
	AttachmentIDs []string `json:"attachment_ids"`
}

// OptionalID preserves the distinction between omitted assignment and explicit null.
type OptionalID struct {
	Present bool
	Value   *string
}

func (o *OptionalID) UnmarshalJSON(data []byte) error {
	o.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type UpdateInput struct {
	Version    int32      `json:"version"`
	Priority   *string    `json:"priority"`
	AssigneeID OptionalID `json:"assignee_id"`
	Status     *string    `json:"status"`
	Reason     *string    `json:"reason"`
}
type VersionInput struct {
	Version int32 `json:"version"`
}
type CommentInput struct {
	Version    int32  `json:"version"`
	Body       string `json:"body"`
	IsInternal bool   `json:"is_internal"`
}
type CommentResult struct {
	Ticket  Ticket  `json:"ticket"`
	Comment Comment `json:"comment"`
}
type AttachmentInput struct {
	Version       int32    `json:"version"`
	AttachmentIDs []string `json:"attachment_ids"`
}
type ReopenInput struct {
	Version int32  `json:"version"`
	Reason  string `json:"reason"`
}
type ArticleInput struct {
	Version       int32    `json:"version"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	CategoryID    string   `json:"category_id"`
	Symptoms      string   `json:"symptoms"`
	SolutionSteps []string `json:"solution_steps"`
	Tags          []string `json:"tags"`
}
type Store interface {
	List(context.Context, auth.User, Filter) (Page, error)
	ExportSnapshot(context.Context, auth.User, Filter, time.Time) (ExportSnapshot, error)
	Detail(context.Context, auth.User, string) (Detail, error)
	Assignees(context.Context, auth.User) ([]Assignee, error)
	Departments(context.Context, auth.User) ([]Department, error)
	Transaction(context.Context, func(MutationStore) error) error
}
type MutationStore interface {
	Lock(context.Context) error
	Actor(context.Context, string) (auth.User, error)
	DefaultPriority(context.Context) (string, error)
	Ticket(context.Context, string) (Ticket, error)
	Get(context.Context, auth.User, string) (Ticket, error)
	Category(context.Context, string) (bool, error)
	PublicArticle(context.Context, string) error
	CheckMedia(context.Context, string, string, bool) error
	Create(context.Context, auth.User, CreateInput) (Ticket, error)
	Update(context.Context, string, string, *string, string, *time.Time) error
	Bump(context.Context, string) error
	LinkArticle(context.Context, string, string) error
	LinkAttachment(context.Context, string, string) error
	Comment(context.Context, string, auth.User, CommentInput) (Comment, error)
	History(context.Context, string, *string, string, string, string) error
	Convert(context.Context, string, string, ArticleInput) (knowledge.Article, error)
	Audit(context.Context, string, string, string, map[string]any) error
}
type KnowledgeService interface {
	List(context.Context, auth.User, knowledge.Filter) (knowledge.ArticlePage, error)
}
