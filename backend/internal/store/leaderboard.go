package store

import (
	"context"
	"fmt"

	"github.com/thdelmas/e-agora/backend/internal/model"
)

// TopByRating returns active subjects ordered for the leaderboard by their
// conservative Glicko-2 rating (rating − 2·RD), so a subject only ranks high
// once its rating is both strong and well-established; ties break on lower RD
// (more evidence) then name for determinism (docs/05-ranking.md §Leaderboard
// ordering). The pool scopes the board to one view over the single global rating
// (docs/10 §4): the deceased are excluded unless pool.IncludeDeceased, and
// Continent / FameTop narrow it to a region or the top fame tier — ranking *within*
// a pool is a filter, not a separate rating. Full subject rows are returned (incl.
// rating/rd/wins/losses/died_at); the handler localizes each.
func (s *Store) TopByRating(ctx context.Context, limit, offset int, pool Pool) ([]model.Subject, error) {
	rows, err := s.pool.Query(ctx, `
		WITH cutoff AS (
			SELECT CASE WHEN $5 THEN
			            coalesce(percentile_cont($6) WITHIN GROUP (ORDER BY global_views), 0)
			       ELSE 0 END AS fame_min
			FROM subjects WHERE active AND ($3 OR died_at IS NULL)
		)
		SELECT id, wikidata_id, canonical_name, available_langs,
		       rating, rd, wins, losses, comparisons, died_at
		FROM subjects
		WHERE active AND ($3 OR died_at IS NULL)
		  AND ($4 = '' OR continent = $4)
		  AND global_views >= (SELECT fame_min FROM cutoff)
		ORDER BY (rating - 2 * rd) DESC, rd ASC, canonical_name ASC
		LIMIT $1 OFFSET $2`,
		limit, offset, pool.IncludeDeceased, pool.Continent, pool.FameTop, pool.FamePct)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()

	var out []model.Subject
	for rows.Next() {
		var s model.Subject
		if err := rows.Scan(&s.ID, &s.WikidataID, &s.CanonicalName, &s.AvailableLangs,
			&s.Rating, &s.RD, &s.Wins, &s.Losses, &s.Comparisons, &s.DiedAt); err != nil {
			return nil, fmt.Errorf("scan leaderboard row: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// TotalVotes returns the all-visitor vote count (a leaderboard headline stat).
func (s *Store) TotalVotes(ctx context.Context) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM votes`).Scan(&n); err != nil {
		return 0, fmt.Errorf("total votes: %w", err)
	}
	return n, nil
}
