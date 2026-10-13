// Package httpx contains bounded parsing shared by feature handlers.
package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

const MaxJSONBytes = 64 << 10

type InputError struct {
	Status  int
	Code    string
	Message string
}

func (e *InputError) Error() string { return e.Message }

// Decode accepts one UTF-8 JSON object, rejects unknown fields and bounds memory.
func Decode(w http.ResponseWriter, r *http.Request, destination any) error {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return &InputError{http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json"}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxJSONBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &InputError{http.StatusRequestEntityTooLarge, "body_too_large", "Request body is too large"}
		}
		return invalidJSON()
	}
	body = bytes.TrimSpace(body)
	if !utf8.Valid(body) || len(body) == 0 || body[0] != '{' {
		return invalidJSON()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalidJSON()
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return invalidJSON()
	}
	return nil
}

func invalidJSON() error {
	return &InputError{http.StatusBadRequest, "invalid_json", "A valid JSON object with supported fields is required"}
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var input *InputError
	if errors.As(err, &input) {
		response.Error(w, r, input.Status, input.Code, input.Message)
		return
	}
	response.Error(w, r, http.StatusBadRequest, "invalid_input", "Invalid request input")
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidUUID(value string) bool { return uuidPattern.MatchString(value) }

func ValidText(value string, maximum int) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(strings.TrimSpace(value)) < 1 || utf8.RuneCountInString(value) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

type Pagination struct {
	Limit  int32
	Offset int32
	Query  string
	Role   string
	Active *bool
}

func ParsePagination(r *http.Request) (Pagination, error) {
	values := r.URL.Query()
	p := Pagination{Limit: 20}
	for key, entries := range values {
		if len(entries) != 1 {
			return p, &InputError{400, "invalid_filter", "Filters must not be repeated"}
		}
		switch key {
		case "limit", "offset", "q", "role", "is_active":
		default:
			return p, &InputError{400, "invalid_filter", "Unsupported filter"}
		}
	}
	if value := values.Get("limit"); value != "" {
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 1 || n > 100 {
			return p, &InputError{400, "invalid_pagination", "Limit must be between 1 and 100"}
		}
		p.Limit = int32(n)
	}
	if value := values.Get("offset"); value != "" {
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 0 || n > 100000 {
			return p, &InputError{400, "invalid_pagination", "Offset must be between 0 and 100000"}
		}
		p.Offset = int32(n)
	}
	p.Query = strings.TrimSpace(values.Get("q"))
	if p.Query != "" && !ValidText(p.Query, 100) {
		return p, &InputError{400, "invalid_filter", "Search text must be at most 100 characters"}
	}
	p.Role = values.Get("role")
	if value, present := values["is_active"]; present {
		if value[0] != "true" && value[0] != "false" {
			return p, &InputError{400, "invalid_filter", "is_active must be true or false"}
		}
		active := value[0] == "true"
		p.Active = &active
	}
	return p, nil
}
