package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrInvalidProposal means a proposal referenced a subject that isn't active.
var ErrInvalidProposal = errors.New("store: invalid proposal")

// Belonging smoothing constants (docs/11-belonging-and-proposals.md §4). Additive
// (Laplace) smoothing so a lone 1/1 recall isn't scored 100%: π₀ is the neutral
// prior share a never-recalled subject sits at, a the prior strength (how much
// evidence it takes to move off π₀).
const (
	belongPriorShare    = 0.05 // π₀
	belongPriorStrength = 5.0  // a
)

// PoolKey is the canonical identity of a pool's *scope* for the belonging axis
// (docs/11 §5). Geography only — belonging is about who's *in* a scope, not the
// fame/status view filters layered on top, so those don't change the key. Country
// is the finer scope and wins when both are set (the picker sends one or the
// other). Values: "world" | "continent:Europe" | "country:France".
func PoolKey(p Pool) string {
	switch {
	case p.Country != "":
		return "country:" + p.Country
	case p.Continent != "":
		return "continent:" + p.Continent
	default:
		return "world"
	}
}

// BelongingScore is the smoothed share of a pool's proposals that named this
// subject: (n + a) / (N + a/π₀), where n is the subject's recall count in the
// pool and N the pool's total proposals. A never-recalled subject sits at π₀; a
// consistently-recalled one approaches its true share as evidence (N) grows —
// confidence-weighted, so an early 1/1 doesn't read as certainty.
func BelongingScore(n, poolTotal int) float64 {
	return (float64(n) + belongPriorStrength) /
		(float64(poolTotal) + belongPriorStrength/belongPriorShare)
}

// RecordProposal logs one recall of a subject for a pool (docs/11 §2) and bumps
// the maintained belonging counter and the pool's proposal total — atomically,
// mirroring RecordVote (append-only event + maintained aggregate). The subject
// must be active; a recall of a stale or hidden id is rejected. A proposal is
// deliberately *not* checked against current pool membership: recalling someone
// the geography didn't place here is exactly the signal we want (and, later, the
// hook that grows the pool).
func (s *Store) RecordProposal(ctx context.Context, sessionID, poolKey string, subjectID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	var active bool
	err = tx.QueryRow(ctx, `SELECT active FROM subjects WHERE id = $1`, subjectID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return ErrInvalidProposal
	}
	if err != nil {
		return fmt.Errorf("proposal: check subject: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO proposals (session_id, pool_key, subject_id) VALUES ($1, $2, $3)`,
		sessionID, poolKey, subjectID); err != nil {
		return fmt.Errorf("proposal: insert: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO pool_belonging (pool_key, subject_id, proposals) VALUES ($1, $2, 1)
		 ON CONFLICT (pool_key, subject_id)
		 DO UPDATE SET proposals = pool_belonging.proposals + 1, updated_at = now()`,
		poolKey, subjectID); err != nil {
		return fmt.Errorf("proposal: bump belonging: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO pool_stats (pool_key, proposals) VALUES ($1, 1)
		 ON CONFLICT (pool_key)
		 DO UPDATE SET proposals = pool_stats.proposals + 1, updated_at = now()`,
		poolKey); err != nil {
		return fmt.Errorf("proposal: bump pool total: %w", err)
	}
	return tx.Commit(ctx)
}

// SubjectRef is a minimal subject reference for the recall type-ahead (docs/11 §2).
type SubjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// RecallSearch finds active subjects whose canonical name matches q, for the
// "who comes to mind for this pool?" type-ahead (docs/11 §2). Case-insensitive
// substring; most-compared first so a well-known figure surfaces above an obscure
// namesake. Recall is deliberately *not* pool-scoped — naming someone the
// geography didn't place here is itself the belonging signal. Capped at limit.
func (s *Store) RecallSearch(ctx context.Context, q string, limit int) ([]SubjectRef, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, canonical_name FROM subjects
		 WHERE active AND canonical_name ILIKE '%' || $1 || '%'
		 ORDER BY comparisons DESC, canonical_name ASC
		 LIMIT $2`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("recall search: %w", err)
	}
	defer rows.Close()

	out := []SubjectRef{}
	for rows.Next() {
		var r SubjectRef
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, fmt.Errorf("scan recall: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BelongingEntry is one subject's belonging in a pool: its recall count and the
// smoothed score (docs/11 §4).
type BelongingEntry struct {
	SubjectID int64
	Proposals int
	Score     float64
}

// Belonging returns the subjects recalled for a pool, most-recalled first, with
// their smoothed belonging score against the pool's total proposals. Empty for a
// pool no one has proposed into yet (membership then falls back to the geographic
// seed, docs/11 §4).
func (s *Store) Belonging(ctx context.Context, poolKey string) ([]BelongingEntry, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT coalesce((SELECT proposals FROM pool_stats WHERE pool_key = $1), 0)`,
		poolKey).Scan(&total); err != nil {
		return nil, fmt.Errorf("belonging total: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT subject_id, proposals FROM pool_belonging
		 WHERE pool_key = $1 ORDER BY proposals DESC, subject_id ASC`, poolKey)
	if err != nil {
		return nil, fmt.Errorf("belonging: %w", err)
	}
	defer rows.Close()

	var out []BelongingEntry
	for rows.Next() {
		var e BelongingEntry
		if err := rows.Scan(&e.SubjectID, &e.Proposals); err != nil {
			return nil, fmt.Errorf("scan belonging: %w", err)
		}
		e.Score = BelongingScore(e.Proposals, total)
		out = append(out, e)
	}
	return out, rows.Err()
}
