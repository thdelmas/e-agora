package store

import (
	"context"
	"fmt"
)

// UpsertPageviews records a subject's trailing-window view count for one
// language, replacing any prior row for that (subject, lang). It is the
// per-call sink of the sync's pageview pass
// (docs/10-recognition-and-pools.md §7, M9); the cross-language sum is
// materialized separately by RefreshGlobalViews.
func (s *Store) UpsertPageviews(
	ctx context.Context, subjectID int64, lang string, views int64,
) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO subject_pageviews (subject_id, lang, views, window_end)
		VALUES ($1, $2, $3, CURRENT_DATE)
		ON CONFLICT (subject_id, lang) DO UPDATE SET
			views      = EXCLUDED.views,
			window_end = EXCLUDED.window_end`,
		subjectID, lang, views,
	)
	if err != nil {
		return fmt.Errorf("upsert pageviews %d/%s: %w",
			subjectID, lang, err)
	}
	return nil
}

// RefreshGlobalViews recomputes subjects.global_views from subject_pageviews
// in a single pass — the global-fame lever and the fame-tier pool read this
// column rather than aggregating per draw. The sync calls it once after
// upserting every subject's per-language counts.
func (s *Store) RefreshGlobalViews(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE subjects s SET global_views = COALESCE(
			(SELECT SUM(views) FROM subject_pageviews p `+
		`WHERE p.subject_id = s.id), 0)`); err != nil {
		return fmt.Errorf("refresh global_views: %w", err)
	}
	return nil
}
