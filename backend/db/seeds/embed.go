// Package seeds embeds synthetic reference data for the development-only seed command.
package seeds

import _ "embed"

// DevelopmentSQL contains its own transaction markers for standalone psql usage.
//
//go:embed development.sql
var DevelopmentSQL string

// KnowledgeSQL inserts synthetic guides only after development accounts exist.
//
//go:embed knowledge.sql
var KnowledgeSQL string
