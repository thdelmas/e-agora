package store

import (
	"context"
	"fmt"
)

// UpsertSubject inserts a subject or, on a wikidata_id conflict, refreshes its
// metadata (name, country, available languages) WITHOUT touching rating, wins,
// losses, comparisons, active, or source — so re-seeding (EAGORA_SEED=force)
// never resets ratings or vote history (docs/06-wikipedia-ingestion.md §Step 3).
// Returns the subject's internal id.
func (s *Store) UpsertSubject(ctx context.Context, qid, canonicalName, country, source string, langs []string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO subjects (wikidata_id, canonical_name, country, source, available_langs)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)
		ON CONFLICT (wikidata_id) DO UPDATE SET
			canonical_name  = EXCLUDED.canonical_name,
			country         = EXCLUDED.country,
			available_langs = EXCLUDED.available_langs,
			updated_at      = now()
		RETURNING id`,
		qid, canonicalName, country, source, langs,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert subject %s: %w", qid, err)
	}
	return id, nil
}

// UpsertTranslation caches per-language display content for a subject, replacing
// any prior row for that (subject, lang). description and image_url may be empty
// (stored NULL); wikipedia_url is required (R2).
func (s *Store) UpsertTranslation(ctx context.Context, subjectID int64, lang, name, description, imageURL, wikipediaURL string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO subject_translations (subject_id, lang, name, description, image_url, wikipedia_url, fetched_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, now())
		ON CONFLICT (subject_id, lang) DO UPDATE SET
			name          = EXCLUDED.name,
			description   = EXCLUDED.description,
			image_url     = EXCLUDED.image_url,
			wikipedia_url = EXCLUDED.wikipedia_url,
			fetched_at    = now()`,
		subjectID, lang, name, description, imageURL, wikipediaURL,
	)
	if err != nil {
		return fmt.Errorf("upsert translation %d/%s: %w", subjectID, lang, err)
	}
	return nil
}
