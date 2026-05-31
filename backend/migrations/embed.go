// Package migrations embeds the SQL schema files so the server can apply them on
// startup without external files (docs/02-architecture.md §Migrations). The
// embedded runner that tracks schema_migrations lands in M1.
package migrations

import "embed"

// FS holds all *.sql migrations, applied in lexical order.
//
//go:embed *.sql
var FS embed.FS
