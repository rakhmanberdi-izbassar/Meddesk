// Package settings manages the clinic configuration without exposing deployment secrets.
package settings

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"time"
)

var (
	ErrInvalidInput    = errors.New("invalid settings input")
	ErrForbidden       = errors.New("settings operation forbidden")
	ErrVersionConflict = errors.New("settings version conflict")
)

type Settings struct {
	ClinicName            string    `json:"clinic_name"`
	DefaultTicketPriority string    `json:"default_ticket_priority"`
	ReportingTimezone     string    `json:"reporting_timezone"`
	Version               int32     `json:"version"`
	UpdatedAt             time.Time `json:"updated_at"`
}
type UpdateInput struct {
	Version               int32   `json:"version"`
	ClinicName            *string `json:"clinic_name"`
	DefaultTicketPriority *string `json:"default_ticket_priority"`
	ReportingTimezone     *string `json:"reporting_timezone"`
}
type Store interface {
	Get(context.Context, auth.User) (Settings, error)
	Transaction(context.Context, func(MutationStore) error) error
}
type MutationStore interface {
	Lock(context.Context) error
	Actor(context.Context, string) (auth.User, error)
	Get(context.Context) (Settings, error)
	Update(context.Context, Settings) (Settings, error)
	Audit(context.Context, string, int32, []string) error
}
