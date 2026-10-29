package school

import "testing"

func TestTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		allowed  bool
	}{
		{"NEW", "RESOLVED", false}, {"NEW", "IN_PROGRESS", true}, {"IN_PROGRESS", "WAITING_FOR_USER", true}, {"WAITING_FOR_USER", "RESOLVED", true}, {"RESOLVED", "CLOSED", true}, {"CLOSED", "NEW", false}, {"RESOLVED", "IN_PROGRESS", true}, {"IN_PROGRESS", "NEW", false},
	} {
		if transition(tc.from, tc.to) != tc.allowed {
			t.Fatalf("%s -> %s", tc.from, tc.to)
		}
	}
}
func TestBodyValidation(t *testing.T) {
	for _, v := range []string{"\x00password", "\x7f", string([]byte{0xff}), "   "} {
		if body(v, 100, false) {
			t.Fatalf("accepted %q", v)
		}
	}
	if !body("Камера\nне работает", 100, false) {
		t.Fatal("multilingual content rejected")
	}
}
