package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditMetadataAllowlistedTypedFields(t *testing.T) {
	raw := []byte(`{"role":"ADMIN","department_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","is_active":false,"password_reset":true,"sessions_revoked":true,"password_hash":"secret-hash","token":"secret-token","email":"person@example.com","description":"secret-text"}`)
	out := SanitizeMetadata("user.updated", raw)
	if len(out) != 5 || out["role"] != "ADMIN" || out["is_active"] != false {
		t.Fatalf("valid safe fields lost: %+v", out)
	}
	bytes, err := json.Marshal(out)
	if err != nil || strings.Contains(string(bytes), "secret") || strings.Contains(string(bytes), "email") {
		t.Fatalf("secret metadata leaked: %s %v", bytes, err)
	}
	malicious := []byte(`{"role":"secret-token","department_id":"secret-password","is_active":{"token":"secret-token"},"password_reset":"secret-password","sessions_revoked":["secret-token"]}`)
	if out := SanitizeMetadata("user.updated", malicious); len(out) != 0 {
		t.Fatalf("trusted keys hid secrets: %+v", out)
	}
	for _, raw := range []string{`{"version":"token","status":"password","priority":"secret","assignee_id":"token"}`, `{"version":1.5,"status":{},"priority":[],"assignee_id":{"token":"secret"}}`, `{"version":-1}`, `{"version":9223372036854775808}`} {
		if out := SanitizeMetadata("ticket.updated", []byte(raw)); len(out) != 0 {
			t.Fatalf("invalid primitives returned: %+v", out)
		}
	}
	if out := SanitizeMetadata("unknown.action", []byte(`{"version":1,"priority":"HIGH"}`)); len(out) != 0 {
		t.Fatalf("unknown action trusted: %+v", out)
	}
	out = SanitizeMetadata("ticket.updated", []byte(`{"version":2,"status":"RESOLVED","priority":"HIGH","assignee_id":null,"reason":"secret"}`))
	if len(out) != 4 || out["version"] != int64(2) {
		t.Fatalf("safe ticketmeta lost: %+v", out)
	}
}
func TestSettingsMetadataCannotCarryConfigurationText(t *testing.T) {
	out := SanitizeMetadata("settings.updated", []byte(`{"version":3,"changed_fields":["clinic_name","reporting_timezone"],"clinic_name":"secret","reporting_timezone":"secret"}`))
	if len(out) != 2 {
		t.Fatalf("settings metadata wrong: %+v", out)
	}
	for _, raw := range []string{`{"changed_fields":["token"]}`, `{"changed_fields":["clinic_name",{"token":"secret"}]}`, `{"changed_fields":"secret"}`, `{"changed_fields":["clinic_name","clinic_name","clinic_name","clinic_name"]}`} {
		if out := SanitizeMetadata("settings.updated", []byte(raw)); len(out) != 0 {
			t.Fatalf("changedfields text escaped: %+v", out)
		}
	}
}
