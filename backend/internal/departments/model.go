// Package departments manages clinic departments without deleting account history.
package departments

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput   = errors.New("invalid department input")
	ErrForbidden      = errors.New("department operation forbidden")
	ErrNotFound       = errors.New("department not found")
	ErrConflict       = errors.New("department name already exists")
	ErrHasActiveUsers = errors.New("department still has active users")
)

type Department struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Filter struct {
	Search        string
	Active        *bool
	Limit, Offset int
}
type Page struct {
	Items  []Department `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}
type CreateInput struct {
	Name string `json:"name"`
}
type UpdateInput struct {
	Name     string `json:"name"`
	IsActive *bool  `json:"is_active"`
}
