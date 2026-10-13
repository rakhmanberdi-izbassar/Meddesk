// Package analytics exposes aggregated operational data to support staff.
package analytics

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid analytics filters")
	ErrForbidden    = errors.New("analytics operation forbidden")
)

type Period struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	Timezone string    `json:"timezone"`
	StartAt  time.Time `json:"start_at"`
	EndAt    time.Time `json:"end_at"`
}
type Filter struct{ From, To, Timezone, DepartmentID, CategoryID, AssigneeID string }
type Summary struct {
	TicketsTotal             int64    `json:"tickets_total"`
	TicketsOpen              int64    `json:"tickets_open"`
	TicketsResolved          int64    `json:"tickets_resolved"`
	TicketsClosed            int64    `json:"tickets_closed"`
	AverageResolutionSeconds *float64 `json:"average_resolution_seconds"`
	FeedbackAttempts         int64    `json:"feedback_attempts"`
	HelpfulAttempts          int64    `json:"helpful_attempts"`
	SelfServiceRate          *float64 `json:"self_service_rate"`
}
type Department struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Category struct {
	ID     string `json:"id"`
	NameKK string `json:"name_kk"`
	NameRU string `json:"name_ru"`
}
type Assignee struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}
type DepartmentCount struct {
	Department
	Total    int64 `json:"total"`
	Open     int64 `json:"open"`
	Resolved int64 `json:"resolved"`
}
type CategoryCount struct {
	ID       *string `json:"id"`
	NameKK   string  `json:"name_kk"`
	NameRU   string  `json:"name_ru"`
	Total    int64   `json:"total"`
	Open     int64   `json:"open"`
	Resolved int64   `json:"resolved"`
}
type Article struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	ViewCount       int64  `json:"view_count"`
	HelpfulCount    int64  `json:"helpful_count"`
	NotHelpfulCount int64  `json:"not_helpful_count"`
}
type Trend struct {
	Date             string `json:"date"`
	TicketsCreated   int64  `json:"tickets_created"`
	TicketsResolved  int64  `json:"tickets_resolved"`
	FeedbackAttempts int64  `json:"feedback_attempts"`
	HelpfulAttempts  int64  `json:"helpful_attempts"`
}
type Report struct {
	Period          Period            `json:"period"`
	Summary         Summary           `json:"summary"`
	Departments     []DepartmentCount `json:"departments"`
	Categories      []CategoryCount   `json:"categories"`
	CommonIssues    []CategoryCount   `json:"common_issues"`
	PopularArticles []Article         `json:"popular_articles"`
	Trend           []Trend           `json:"trend"`
}
type Filters struct {
	Departments []Department `json:"departments"`
	Categories  []Category   `json:"categories"`
	Assignees   []Assignee   `json:"assignees"`
}
type Store interface {
	Report(context.Context, auth.User, Filter, time.Time) (Report, error)
	Filters(context.Context, auth.User) (Filters, error)
}
