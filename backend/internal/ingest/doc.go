// Package ingest sources the pool from Wikidata (enumerate UN leaders, "is a
// human" check, sitelinks → available_langs) and Wikipedia REST (per-language
// summaries), and runs the seed-on-startup step honoring EAGORA_SEED
// (docs/06-wikipedia-ingestion.md).
//
// Implemented in M2 (docs/07-roadmap.md).
package ingest
