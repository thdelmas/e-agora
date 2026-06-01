package store

import (
	"context"
	"fmt"
)

// UpsertSubject inserts a subject or, on a wikidata_id conflict, refreshes its
// metadata (name, available languages, date of death) WITHOUT touching rating,
// wins, losses, comparisons, active, or source — so re-seeding (EAGORA_SEED=force)
// never resets ratings or vote history (docs/06-wikipedia-ingestion.md §Step 3).
// diedAt is a normalized YYYY-MM-DD date or "" (stored NULL → living/unknown).
// Returns the subject's internal id.
func (s *Store) UpsertSubject(ctx context.Context, qid, canonicalName, source string, langs []string, diedAt string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO subjects (wikidata_id, canonical_name, source, available_langs, died_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::date)
		ON CONFLICT (wikidata_id) DO UPDATE SET
			canonical_name  = EXCLUDED.canonical_name,
			available_langs = EXCLUDED.available_langs,
			died_at         = EXCLUDED.died_at,
			updated_at      = now()
		RETURNING id`,
		qid, canonicalName, source, langs, diedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert subject %s: %w", qid, err)
	}
	return id, nil
}

// AllSubjectQIDs returns every subject's Wikidata QID (active or not, seed or
// user-added), oldest first. It is the candidate set the daily sync re-fetches
// from Wikidata to refresh metadata (name, languages, date of death).
func (s *Store) AllSubjectQIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT wikidata_id FROM subjects ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("all subject qids: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var qid string
		if err := rows.Scan(&qid); err != nil {
			return nil, fmt.Errorf("scan qid: %w", err)
		}
		out = append(out, qid)
	}
	return out, rows.Err()
}

// UpsertTranslation caches per-language display content for a subject, replacing
// any prior row for that (subject, lang). description, extract and image_url may
// be empty (stored NULL); wikipedia_url is required (R2).
func (s *Store) UpsertTranslation(ctx context.Context, subjectID int64, lang, name, description, extract, imageURL, wikipediaURL string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO subject_translations (subject_id, lang, name, description, extract, image_url, wikipedia_url, fetched_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, now())
		ON CONFLICT (subject_id, lang) DO UPDATE SET
			name          = EXCLUDED.name,
			description   = EXCLUDED.description,
			extract       = EXCLUDED.extract,
			image_url     = EXCLUDED.image_url,
			wikipedia_url = EXCLUDED.wikipedia_url,
			fetched_at    = now()`,
		subjectID, lang, name, description, extract, imageURL, wikipediaURL,
	)
	if err != nil {
		return fmt.Errorf("upsert translation %d/%s: %w", subjectID, lang, err)
	}
	return nil
}
