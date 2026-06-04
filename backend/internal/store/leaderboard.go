package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/thdelmas/e-agora/backend/internal/model"
)

// TopByRating returns active subjects ordered for the leaderboard by their
// conservative Glicko-2 rating (rating − 2·RD), so a subject only ranks high
// once its rating is both strong and well-established; ties break on lower RD
// (more evidence) then name for determinism (docs/05-ranking.md §Leaderboard
// ordering). The pool scopes the board to one view over the single global
// rating (docs/10 §4): the deceased are excluded unless pool.IncludeDeceased,
// and Continent / Country / FameTop narrow it to a region, a country or the
// top fame tier — ranking *within* a pool is a filter, not a separate rating.
// Full subject rows are returned (incl. rating/rd/wins/losses/died_at); the
// handler localizes each.
func (s *Store) TopByRating(
	ctx context.Context, limit, offset int, pool Pool,
) ([]model.Subject, error) {
	rows, err := s.pool.Query(ctx, `
		WITH cutoff AS (
			SELECT CASE WHEN $5 THEN
			            coalesce(percentile_cont($6) `+
		`WITHIN GROUP (ORDER BY global_views), 0)
			       ELSE 0 END AS fame_min
			FROM subjects WHERE active AND ($3 OR died_at IS NULL)
		)
		SELECT id, wikidata_id, canonical_name, available_langs,
		       rating, rd, wins, losses, comparisons, died_at
		FROM subjects
		WHERE active AND ($3 OR died_at IS NULL)
		  AND ($4 = '' OR continent @> ARRAY[$4])
		  AND ($7 = '' OR country = $7)
		  AND global_views >= (SELECT fame_min FROM cutoff)
		ORDER BY (rating - 2 * rd) DESC, rd ASC, canonical_name ASC
		LIMIT $1 OFFSET $2`,
		limit, offset, pool.IncludeDeceased, pool.Continent, pool.FameTop,
		pool.FamePct, pool.Country)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()

	var out []model.Subject
	for rows.Next() {
		var s model.Subject
		if err := rows.Scan(&s.ID, &s.WikidataID, &s.CanonicalName,
			&s.AvailableLangs, &s.Rating, &s.RD, &s.Wins, &s.Losses,
			&s.Comparisons, &s.DiedAt); err != nil {
			return nil, fmt.Errorf("scan leaderboard row: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SubjectWithRank returns one active subject's full row plus its rank within
// the given pool, for the dedicated subject view reached by clicking a
// leaderboard row. The rank mirrors TopByRating exactly — same conservative
// ordering (rating − 2·RD) and the same pool filters
// (deceased/continent/country/fame) — so the number on the detail page matches
// the board the visitor came from. found is false when no active subject has
// that id. The handler localizes the row, as it does for the leaderboard.
func (s *Store) SubjectWithRank(
	ctx context.Context, id int64, pool Pool,
) (model.Subject, int, bool, error) {
	var subj model.Subject
	var rank int
	err := s.pool.QueryRow(ctx, `
		WITH cutoff AS (
			SELECT CASE WHEN $4 THEN
			            coalesce(percentile_cont($5) `+
		`WITHIN GROUP (ORDER BY global_views), 0)
			       ELSE 0 END AS fame_min
			FROM subjects WHERE active AND ($2 OR died_at IS NULL)
		)
		SELECT s.id, s.wikidata_id, s.canonical_name, s.available_langs,
		       s.rating, s.rd, s.wins, s.losses, s.comparisons, s.died_at,
		       (SELECT count(*) FROM subjects o
		        WHERE o.active AND ($2 OR o.died_at IS NULL)
		          AND ($3 = '' OR o.continent @> ARRAY[$3])
		          AND ($6 = '' OR o.country = $6)
		          AND o.global_views >= (SELECT fame_min FROM cutoff)
		          AND (o.rating - 2 * o.rd) > (s.rating - 2 * s.rd)) + 1
		FROM subjects s
		WHERE s.id = $1 AND s.active`,
		id, pool.IncludeDeceased, pool.Continent, pool.FameTop,
		pool.FamePct, pool.Country).
		Scan(&subj.ID, &subj.WikidataID, &subj.CanonicalName,
			&subj.AvailableLangs, &subj.Rating, &subj.RD, &subj.Wins,
			&subj.Losses, &subj.Comparisons, &subj.DiedAt, &rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Subject{}, 0, false, nil
	}
	if err != nil {
		return model.Subject{}, 0, false, fmt.Errorf("subject with rank: %w", err)
	}
	return subj, rank, true, nil
}

// TotalVotes returns the all-visitor vote count (a leaderboard headline stat).
func (s *Store) TotalVotes(ctx context.Context) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM votes`).Scan(&n); err != nil {
		return 0, fmt.Errorf("total votes: %w", err)
	}
	return n, nil
}
