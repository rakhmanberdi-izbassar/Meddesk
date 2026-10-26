package bootstrap

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeBootstrapInput(t *testing.T) {
	valid := Input{Email: " ADMIN@example.invalid ", DisplayName: " Әкімші ", Department: " IT ", Password: "synthetic-password"}
	normalized, err := normalize(valid)
	if err != nil || normalized.Email != "admin@example.invalid" || normalized.DisplayName != "Әкімші" || normalized.Department != "IT" || normalized.Password != valid.Password {
		t.Fatal("administrator fields were not normalized correctly")
	}
	for _, change := range []func(*Input){
		func(i *Input) { i.Email = "secret-password invalid email" },
		func(i *Input) { i.DisplayName = "" },
		func(i *Input) { i.Department = "bad\x00department" },
		func(i *Input) { i.DisplayName = strings.Repeat("x", 151) },
		func(i *Input) { i.Password = "short" },
		func(i *Input) { i.Password = strings.Repeat("x", 73) },
		func(i *Input) { i.Password = strings.Repeat("x", 12) + "\xff" },
	} {
		input := valid
		change(&input)
		if _, err := normalize(input); !errors.Is(err, ErrInvalidInput) || strings.Contains(err.Error(), "secret-password") {
			t.Fatal("invalid bootstrap input accepted or leaked")
		}
	}
}
