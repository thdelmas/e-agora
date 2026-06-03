// Package subjects implements visitor-added subjects (R8): token-gated and
// limited to one add per token (R8.1), resolving input to a Wikidata QID,
// asserting "is a human" + a real Wikipedia page, deduping by QID, and
// ingesting (docs/04-api.md POST /api/subjects,
// docs/06-wikipedia-ingestion.md).
//
// Implemented in M4 (docs/07-roadmap.md).
package subjects
