// Package migrations embeds the ordered PostgreSQL schema migrations into the
// API binary so every deployment applies the same versioned schema.
package migrations

import "embed"

// Files contains every numbered SQL migration.
//
//go:embed *.sql
var Files embed.FS
