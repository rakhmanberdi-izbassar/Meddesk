package media

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type AuthorizationGuard interface {
	Protect(http.Handler, ...string) http.Handler
}
type HTTPService interface {
	Maximum(string) (int64, error)
	Upload(context.Context, auth.User, UploadInput) (File, error)
	Download(context.Context, auth.User, string) (File, *os.File, error)
}
type Handler struct {
	service HTTPService
	guard   AuthorizationGuard
}

func NewHandler(service HTTPService, guard AuthorizationGuard) *Handler {
	return &Handler{service, guard}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/media", h.guard.Protect(http.HandlerFunc(h.upload)))
	mux.Handle("GET /api/v1/media/{id}", h.guard.Protect(http.HandlerFunc(h.download)))
	mux.Handle("HEAD /api/v1/media/{id}", h.guard.Protect(http.HandlerFunc(h.download)))
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	values, err := parsePurpose(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if actor.Role != auth.RoleAdmin && actor.Role != auth.RoleEngineer &&
		(actor.Role != auth.RoleEmployee || values != PurposeTicket) {
		writeError(w, r, ErrForbidden)
		return
	}
	maximum, err := h.service.Maximum(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "multipart/form-data" || params["boundary"] == "" {
		response.Error(w, r, 415, "unsupported_media_type", "A multipart/form-data upload is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximum+(64<<10))
	reader := multipart.NewReader(r.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		writeError(w, r, uploadReadError(err))
		return
	}
	defer part.Close()
	disposition, attributes, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil || disposition != "form-data" || attributes["name"] != "file" || attributes["filename"] == "" || len(part.Header.Values("Content-Type")) > 1 {
		writeError(w, r, ErrInvalidInput)
		return
	}
	file, err := h.service.Upload(r.Context(), actor, UploadInput{Purpose: values, Filename: attributes["filename"], ContentType: part.Header.Get("Content-Type"), Reader: &singlePartReader{part: part, reader: reader}})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, 201, map[string]File{"media": file})
}
func parsePurpose(r *http.Request) (string, error) {
	values := r.URL.Query()
	if len(values) != 1 || len(values["purpose"]) != 1 {
		return "", ErrInvalidInput
	}
	purpose := values.Get("purpose")
	if purpose != PurposeImage && purpose != PurposeVideo && purpose != PurposeTicket {
		return "", ErrInvalidInput
	}
	return purpose, nil
}

// The closing multipart boundary is checked by the same stream consumed by
// Upload, so a duplicate part or malformed trailer cannot commit metadata.
type singlePartReader struct {
	part   *multipart.Part
	reader *multipart.Reader
	done   bool
}

func (s *singlePartReader) Read(b []byte) (int, error) {
	if s.done {
		return 0, io.EOF
	}
	n, err := s.part.Read(b)
	if errors.Is(err, io.EOF) {
		s.done = true
		next, nextErr := s.reader.NextPart()
		if next != nil {
			_ = next.Close()
		}
		if !errors.Is(nextErr, io.EOF) {
			return n, uploadReadError(nextErr)
		}
		return n, io.EOF
	}
	if err != nil {
		return n, uploadReadError(err)
	}
	return n, nil
}
func uploadReadError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return ErrTooLarge
	}
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrInvalidInput
	}
	return ErrInvalidInput
}
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, 401, "unauthenticated", "Authentication required")
		return
	}
	file, stream, err := h.service.Download(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.OriginalFilename}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Cookie")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("ETag", `"`+file.ID+`"`)
	http.ServeContent(w, r, file.OriginalFilename, file.CreatedAt, stream)
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		response.Error(w, r, 400, "invalid_input", "Supply one file and a supported media purpose")
	case errors.Is(err, ErrInvalidFile):
		response.Error(w, r, 415, "invalid_file", "Use a valid PNG, JPEG, MP4 or WebM file matching its content type")
	case errors.Is(err, ErrTooLarge):
		response.Error(w, r, 413, "file_too_large", "The media upload exceeds its configured size limit")
	case errors.Is(err, ErrForbidden):
		response.Error(w, r, 403, "forbidden", "This media operation is not permitted")
	case errors.Is(err, ErrNotFound):
		response.Error(w, r, 404, "not_found", "Media not found")
	default:
		response.Error(w, r, 500, "internal_error", "Unable to complete the media operation")
	}
}
