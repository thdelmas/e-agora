// Package data embeds the backend's bundled seed inputs so the compiled binary
// is self-contained — no data directory needs shipping alongside it
// (docs/06-wikipedia-ingestion.md). Files are read via the exported FS.
package data

import "embed"

// FS holds the committed seed snapshots and prompt pool:
//   - un_leaders.json     Wikidata snapshot of UN-country leaders (seed input)
//   - seed_extra.json     optional hand-picked humans
//   - humanity_prompts.json  R12 prompt pool (used by the human package, M3)
//
//go:embed un_leaders.json seed_extra.json humanity_prompts.json
var FS embed.FS
