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
// ordering). Full subject rows are returned (incl. rating/rd/wins/losses); the
// handler localizes each.
func (s *Store) TopByRating(ctx context.Context, limit, offset int) ([]model.Subject, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, wikidata_id, canonical_name, COALESCE(country, ''), available_langs,
		       rating, rd, wins, losses, comparisons
		FROM subjects WHERE active
		ORDER BY (rating - 2 * rd) DESC, rd ASC, canonical_name ASC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()

	var out []model.Subject
	for rows.Next() {
		var s model.Subject
		if err := rows.Scan(&s.ID, &s.WikidataID, &s.CanonicalName, &s.Country, &s.AvailableLangs,
			&s.Rating, &s.RD, &s.Wins, &s.Losses, &s.Comparisons); err != nil {
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
