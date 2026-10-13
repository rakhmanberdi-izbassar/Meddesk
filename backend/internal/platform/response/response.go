// Package response provides the API's consistent JSON response format.
package response

import (
	"encoding/json"
	"net/http"

	"github.com/meddesk/meddesk/backend/internal/platform/requestmeta"
)

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// All response DTOs contain only JSON-safe values; clients can disconnect during writes.
	_ = json.NewEncoder(w).Encode(value)
}

func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	JSON(w, status, ErrorEnvelope{Error: ErrorDetail{code, message, requestmeta.ID(r.Context())}})
}
