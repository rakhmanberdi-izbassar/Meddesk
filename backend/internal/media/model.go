// Package media owns authenticated media storage and delivery.
package media

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
)

const (
	PurposeImage               = "ARTICLE_IMAGE"
	PurposeVideo               = "ARTICLE_VIDEO"
	PurposeTicket              = "TICKET_ATTACHMENT"
	DefaultMaxImageBytes int64 = 10 << 20
	DefaultMaxVideoBytes int64 = 100 << 20
)

var (
	ErrInvalidInput = errors.New("invalid media input")
	ErrInvalidFile  = errors.New("unsupported or invalid media file")
	ErrTooLarge     = errors.New("media file exceeds upload limit")
	ErrForbidden    = errors.New("media operation forbidden")
	ErrNotFound     = errors.New("media not found")
	// A transport error during COMMIT does not prove PostgreSQL rolled back.
	// Retain the private file until operators reconcile database/storage state.
	ErrCommitUncertain = errors.New("media transaction commit result uncertain")
)

// File excludes private storage paths, hashes and uploader details from JSON.
type File struct {
	ID               string    `json:"id"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	ByteCount        int64     `json:"byte_count"`
	Purpose          string    `json:"purpose"`
	CreatedAt        time.Time `json:"created_at"`
	URL              string    `json:"url"`
	StorageKey       string    `json:"-"`
	SHA256           []byte    `json:"-"`
}

type Options struct{ MaxImageBytes, MaxVideoBytes int64 }
type UploadInput struct {
	Purpose, Filename, ContentType string
	Reader                         io.Reader
}
type Store interface {
	Create(context.Context, auth.User, File) (File, error)
	Find(context.Context, auth.User, string) (File, error)
}
