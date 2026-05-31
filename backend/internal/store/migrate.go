package store

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Migrate applies every *.sql file in fsys whose version isn't yet recorded in
// schema_migrations, in ascending version order, each within its own
// transaction. It is idempotent: a re-run with nothing new applies zero
// migrations. The runner owns the schema_migrations bookkeeping table — it
// bootstraps the table before reading it, resolving the chicken-and-egg where
// the first migration would otherwise need the table to record itself
// (docs/02-architecture.md §Migrations). Returns the number applied.
func (s *Store) Migrate(ctx context.Context, fsys fs.FS) (int, error) {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return 0, fmt.Errorf("bootstrap schema_migrations: %w", err)
	}

	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}

	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return 0, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	count := 0
	for _, name := range names {
		version, err := versionOf(name)
		if err != nil {
			return count, err
		}
		if applied[version] {
			continue
		}
		if err := s.applyOne(ctx, fsys, name, version); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// appliedVersions returns the set of migration versions already recorded.
func (s *Store) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	done := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}

// applyOne runs a single migration file and records its version atomically.
func (s *Store) applyOne(ctx context.Context, fsys fs.FS, name string, version int) error {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}

// versionOf parses the leading integer from a "<version>_<name>.sql" filename.
func versionOf(name string) (int, error) {
	base := path.Base(name)
	idx := strings.IndexByte(base, '_')
	if idx <= 0 {
		return 0, fmt.Errorf("migration %q must be named <version>_<name>.sql", name)
	}
	v, err := strconv.Atoi(base[:idx])
	if err != nil {
		return 0, fmt.Errorf("migration %q has a non-numeric version: %w", name, err)
	}
	return v, nil
}
