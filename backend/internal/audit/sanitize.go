package audit

import (
	"bytes"
	"encoding/json"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"strconv"
)

var fieldsByAction = map[string][]string{
	"user.created":     {"role", "department_id"},
	"user.updated":     {"role", "department_id", "is_active", "password_reset", "sessions_revoked"},
	"article.created":  {"version", "is_published", "category_id", "source_ticket_id"},
	"article.updated":  {"version", "is_published", "category_id"},
	"article.deleted":  {"version"},
	"category.created": {"is_active"}, "category.updated": {"is_active"},
	"ticket.created":           {"version", "priority", "status"},
	"ticket.updated":           {"version", "status", "priority", "assignee_id"},
	"ticket.commented":         {"version", "comment_id", "is_internal"},
	"ticket.attachments_added": {"version", "attachment_count"},
	"ticket.status_changed":    {"version", "from_status", "to_status"},
	"ticket.converted":         {"version", "article_id"},
	"settings.updated":         {"version", "changed_fields"},
}

func member(value string, allowed ...string) bool {
	for _, s := range allowed {
		if value == s {
			return true
		}
	}
	return false
}

// SanitizeMetadata trusts neither historical writers nor familiar field names.
// Known actions may expose only correctly typed enums, IDs, counters and flags.
func SanitizeMetadata(action string, data []byte) map[string]any {
	out := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var source map[string]any
	if dec.Decode(&source) != nil {
		return out
	}
	for _, key := range fieldsByAction[action] {
		value, present := source[key]
		if !present {
			continue
		}
		switch key {
		case "is_active", "password_reset", "sessions_revoked", "is_published", "is_internal":
			if v, ok := value.(bool); ok {
				out[key] = v
			}
		case "version", "attachment_count":
			if n, ok := value.(json.Number); ok {
				v, err := strconv.ParseInt(string(n), 10, 64)
				if err == nil && v >= 0 && (key != "version" || v > 0) {
					out[key] = v
				}
			}
		case "department_id", "category_id", "source_ticket_id", "comment_id", "article_id", "assignee_id":
			if v, ok := value.(string); ok && httpx.ValidUUID(v) {
				out[key] = v
			} else if value == nil && key == "assignee_id" {
				out[key] = nil
			}
		case "role":
			if v, ok := value.(string); ok && member(v, "EMPLOYEE", "IT_ENGINEER", "ADMIN") {
				out[key] = v
			}
		case "priority":
			if v, ok := value.(string); ok && member(v, "LOW", "MEDIUM", "HIGH", "CRITICAL") {
				out[key] = v
			}
		case "status", "from_status", "to_status":
			if v, ok := value.(string); ok && member(v, "NEW", "IN_PROGRESS", "WAITING_FOR_USER", "RESOLVED", "CLOSED") {
				out[key] = v
			}
		case "changed_fields":
			if values, ok := value.([]any); ok && len(values) <= 3 {
				clean := make([]string, 0, len(values))
				valid := true
				for _, item := range values {
					v, ok := item.(string)
					if !ok || !member(v, "clinic_name", "default_ticket_priority", "reporting_timezone") {
						valid = false
						break
					}
					if !member(v, clean...) {
						clean = append(clean, v)
					}
				}
				if valid {
					out[key] = clean
				}
			}
		}
	}
	return out
}
