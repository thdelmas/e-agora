// Package store owns PostgreSQL access via pgxpool: the connection pool, the
// embedded migration runner, and typed queries for subjects, translations,
// votes, sessions, and the add ledger (docs/03-data-model.md).
//
// M1 provides the pool (Open/Close), the migration runner (Migrate), and the
// health count (CountSubjects). Feature-specific queries land with their
// milestones (M2–M4, docs/07-roadmap.md).
package store
