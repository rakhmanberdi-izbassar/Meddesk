package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPasswordStdinBoundedAndPreservesSpaces(t *testing.T) {
	for _, tc := range []struct {
		input, password string
		rejected        bool
	}{
		{"long-synthetic-password\n", "long-synthetic-password", false},
		{"  synthetic password  \n", "  synthetic password  ", false},
		{strings.Repeat("x", 72) + "\n", strings.Repeat("x", 72), false},
		{"", "", true},
		{"short\n", "", true},
		{strings.Repeat("x", 73) + "\n", "", true},
		{strings.Repeat("x", 100000), "", true},
	} {
		password, err := readPassword(strings.NewReader(tc.input))
		if (err != nil) != tc.rejected || password != tc.password {
			t.Fatal("incorrect bounded stdin password handling")
		}
	}
}

func TestBootstrapRequiresOperatorConfirmation(t *testing.T) {
	var output, errorOutput bytes.Buffer
	status := run(nil, strings.NewReader("synthetic-password\n"), &output, &errorOutput)
	if status != 1 || !strings.Contains(errorOutput.String(), "-confirm-empty-database") || strings.Contains(errorOutput.String(), "synthetic-password") || output.Len() != 0 {
		t.Fatal("missing confirmation was accepted or exposed credentials")
	}
}
