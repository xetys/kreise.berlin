// Package migrations embeds the SQL migration files so they can be applied
// in-process by the migration CLI and tests, and exposes the directory path
// for sqlc and the goose CLI.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
