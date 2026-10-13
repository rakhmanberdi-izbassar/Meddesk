package media

import (
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
)

type Service struct {
	store   Store
	files   *FileStore
	options Options
}

func NewService(store Store, files *FileStore, options Options) *Service {
	if options.MaxImageBytes <= 0 || options.MaxImageBytes > DefaultMaxImageBytes {
		options.MaxImageBytes = DefaultMaxImageBytes
	}
	if options.MaxVideoBytes <= 0 || options.MaxVideoBytes > 500<<20 {
		options.MaxVideoBytes = DefaultMaxVideoBytes
	}
	return &Service{store, files, options}
}
func (s *Service) Maximum(purpose string) (int64, error) {
	switch purpose {
	case PurposeImage, PurposeTicket:
		return s.options.MaxImageBytes, nil
	case PurposeVideo:
		return s.options.MaxVideoBytes, nil
	default:
		return 0, ErrInvalidInput
	}
}
func validFilename(name string) bool {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 255 || name == "." || name == ".." || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Service) Upload(ctx context.Context, actor auth.User, input UploadInput) (File, error) {
	if !actor.IsActive || (actor.Role != auth.RoleAdmin && actor.Role != auth.RoleEngineer &&
		(actor.Role != auth.RoleEmployee || input.Purpose != PurposeTicket)) {
		return File{}, ErrForbidden
	}
	maximum, err := s.Maximum(input.Purpose)
	if err != nil {
		return File{}, err
	}
	if !httpx.ValidUUID(actor.ID) || !validFilename(input.Filename) || input.Reader == nil {
		return File{}, ErrInvalidInput
	}
	staged, err := s.files.stage(ctx, input.Reader, maximum)
	if err != nil {
		return File{}, err
	}
	defer s.files.discard(staged)
	contentType, extension, err := validateFile(ctx, staged.file, staged.size, input.Purpose)
	if err != nil {
		return File{}, err
	}
	if input.ContentType != "" {
		declared, _, err := mime.ParseMediaType(input.ContentType)
		if err != nil || (declared != contentType && declared != "application/octet-stream") {
			return File{}, ErrInvalidFile
		}
	}
	key, err := s.files.finalize(ctx, staged, extension)
	if err != nil {
		return File{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.files.Remove(key)
		}
	}()
	file, err := s.store.Create(ctx, actor, File{OriginalFilename: input.Filename, ContentType: contentType, ByteCount: staged.size, Purpose: input.Purpose, StorageKey: key, SHA256: staged.digest})
	if err != nil {
		if errors.Is(err, ErrCommitUncertain) {
			committed = true
		}
		return File{}, err
	}
	committed = true
	return file, nil
}
func (s *Service) Download(ctx context.Context, actor auth.User, id string) (File, *os.File, error) {
	if !actor.IsActive || !httpx.ValidUUID(actor.ID) {
		return File{}, nil, ErrForbidden
	}
	if !httpx.ValidUUID(id) {
		return File{}, nil, ErrInvalidInput
	}
	file, err := s.store.Find(ctx, actor, strings.ToLower(id))
	if err != nil {
		return File{}, nil, err
	}
	stream, err := s.files.Open(file.StorageKey)
	if err != nil {
		if isMissing(err) {
			err = ErrNotFound
		}
		return File{}, nil, err
	}
	stat, err := stream.Stat()
	if err != nil {
		stream.Close()
		return File{}, nil, err
	}
	if stat.Size() != file.ByteCount {
		stream.Close()
		return File{}, nil, ErrNotFound
	}
	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		stream.Close()
		return File{}, nil, err
	}
	return file, stream, nil
}
