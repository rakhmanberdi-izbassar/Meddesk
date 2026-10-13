// Package audit provides a restricted, sanitized view of durable management events.
package audit

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid audit filters")
	ErrForbidden    = errors.New("audit operation forbidden")
)

type Filter struct {
	From, To, Timezone, ActorID, Action, EntityType, EntityID string
	Limit, Offset                                             int
}
type Entry struct {
	ID         int64          `json:"id"`
	ActorID    *string        `json:"actor_id"`
	ActorName  *string        `json:"actor_name"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   *string        `json:"entity_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}
type Page struct {
	Period analytics.Period `json:"period"`
	Items  []Entry          `json:"items"`
	Total  int64            `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}
type Actor struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}
type Filters struct {
	Actions     []string `json:"actions"`
	EntityTypes []string `json:"entity_types"`
	Actors      []Actor  `json:"actors"`
}
type Store interface {
	List(context.Context, auth.User, Filter, time.Time) (Page, error)
	Filters(context.Context, auth.User) (Filters, error)
}
