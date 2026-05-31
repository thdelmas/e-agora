// Package store owns PostgreSQL access via pgxpool: the connection pool, the
// embedded migration runner, and typed queries for subjects, translations,
// votes, sessions, and the add ledger (docs/03-data-model.md).
//
// Implemented in M1 (docs/07-roadmap.md).
package store
