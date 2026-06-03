package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgx connection pool and exposes typed queries.
type Store struct {
	pool *pgxpool.Pool
}

// Open parses the DSN, builds a pgx pool, and verifies connectivity with a
// bounded ping so startup fails loudly when PostgreSQL is unreachable
// (docs/02-architecture.md §Deployment shape).
func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close releases all pooled connections.
func (s *Store) Close() {
	s.pool.Close()
}

// CountSubjects returns the number of rows in subjects (drives /api/healthz).
func (s *Store) CountSubjects(ctx context.Context) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM subjects`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count subjects: %w", err)
	}
	return n, nil
}
