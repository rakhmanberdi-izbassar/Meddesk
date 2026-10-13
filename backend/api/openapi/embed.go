// Package openapi embeds the reviewed API contract in the executable.
package openapi

import _ "embed"

//go:embed openapi.json
var Document []byte
