package seed

import (
	"strings"
	"testing"
)

func TestDemoAccountConfigurationGate(t *testing.T) {
	get := func(string) string { return "Synthetic-fixture-password" }
	for _, environment := range []string{"", "production", "test"} {
		if _, err := Accounts(environment, get); err == nil {
			t.Fatal("seed allowed outside development")
		}
	}
	accounts, err := Accounts("development", get)
	if err != nil || len(accounts) != 3 {
		t.Fatalf("incorrect seed account configuration: %v", err)
	}
	for _, password := range []string{"", "short", strings.Repeat("a", 73)} {
		if _, err := Accounts("development", func(string) string { return password }); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
}
