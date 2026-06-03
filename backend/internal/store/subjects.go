package store

import (
	"context"
	"fmt"
)

// UpsertSubject inserts a subject or, on a wikidata_id conflict, refreshes its
// metadata (name, available languages, date of death) WITHOUT touching rating,
// wins, losses, comparisons, active, or source — so re-seeding
// (EAGORA_SEED=force) never resets ratings or vote history
// (docs/06-wikipedia-ingestion.md §Step 3). diedAt is a normalized YYYY-MM-DD
// date or "" (stored NULL → living/unknown). Returns the subject's internal
// id.
func (s *Store) UpsertSubject(
	ctx context.Context, qid, canonicalName, source string,
	langs []string, diedAt string,
) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO subjects `+
		`(wikidata_id, canonical_name, source, available_langs, died_at)
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

// SetSubjectGeo records a subject's country and continents (the region pool
// axis, docs/10 §4), resolved from Wikidata at sync. continents is the set of
// continent buckets the subject's country belongs to — usually one, two for
// the contiguous transcontinental states (so e.g. a Russian figure matches
// both Europe and Asia). Keyed by Wikidata QID so the startup backfill can
// write geo with only the QID in hand (no internal id lookup). An empty
// country / continent set stores NULL (unknown → the subject matches no
// region pool). Kept separate from UpsertSubject so the geo backfill doesn't
// touch the rest of the upsert path.
func (s *Store) SetSubjectGeo(
	ctx context.Context, qid, country string, continents []string,
) error {
	if len(continents) == 0 {
		continents = nil // store NULL, not an empty array, for "no region"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE subjects `+
		`SET country = NULLIF($2, ''), continent = $3, updated_at = now()
		WHERE wikidata_id = $1`,
		qid, country, continents,
	)
	if err != nil {
		return fmt.Errorf("set subject geo %s: %w", qid, err)
	}
	return nil
}

// SubjectQIDsMissingGeo returns the QIDs of subjects with no resolved
// continent — the work-list for the startup geo backfill (docs/10 §4). A
// deploy that adds the pools feature to an existing pool runs migration 0007
// (an empty continent column) but auto-seed short-circuits a populated pool,
// so without a backfill the region pools stay empty until the daily sync
// happens to run. Returns rows with continent IS NULL (country may already be
// set but continent unresolved); the genuinely unresolvable residue is small
// and simply re-queried each boot.
func (s *Store) SubjectQIDsMissingGeo(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT wikidata_id FROM subjects WHERE continent IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("subjects missing geo: %w", err)
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

// CountryStat is one country present in the pool: its English label (the exact
// value Pool.Country filters on), the continent bucket(s) it belongs to (so the
// picker can group it), and how many active living subjects it has.
type CountryStat struct {
	Name       string   `json:"name"`
	Continents []string `json:"continents"`
	Count      int      `json:"count"`
}

// Countries lists the countries with at least two active living subjects —
// the finer-grained region pools (docs/10 §4) the picker offers beneath the
// continents. country is the Wikidata English label written at geo backfill,
// so each Name is exactly what Pool.Country matches; the two-subject floor
// hides countries too thin to draw a matchup from. Ordered by subject count
// (the pools a visitor is likeliest to want) then name.
func (s *Store) Countries(ctx context.Context) ([]CountryStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT country, continents, cnt FROM (
			SELECT DISTINCT ON (country)
			       country,
			       coalesce(continent, '{}') AS continents,
			       count(*) OVER (PARTITION BY country)::int AS cnt
			FROM subjects
			WHERE active AND died_at IS NULL AND country IS NOT NULL
			ORDER BY country
		) q
		WHERE cnt >= 2
		ORDER BY cnt DESC, country ASC`)
	if err != nil {
		return nil, fmt.Errorf("countries: %w", err)
	}
	defer rows.Close()

	var out []CountryStat
	for rows.Next() {
		var c CountryStat
		if err := rows.Scan(&c.Name, &c.Continents, &c.Count); err != nil {
			return nil, fmt.Errorf("scan country: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AllSubjectQIDs returns every subject's Wikidata QID (active or not, seed or
// user-added), oldest first. It is the candidate set the daily sync re-fetches
// from Wikidata to refresh metadata (name, languages, date of death).
func (s *Store) AllSubjectQIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT wikidata_id FROM subjects ORDER BY id`)
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

// UpsertTranslation caches per-language display content for a subject,
// replacing any prior row for that (subject, lang). description, extract and
// image_url may be empty (stored NULL); wikipedia_url is required (R2).
func (s *Store) UpsertTranslation(
	ctx context.Context, subjectID int64,
	lang, name, description, extract, imageURL, wikipediaURL string,
) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO subject_translations `+
		`(subject_id, lang, name, description, extract, image_url, `+
		`wikipedia_url, fetched_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), `+
		`NULLIF($6, ''), $7, now())
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
		return fmt.Errorf(
			"upsert translation %d/%s: %w", subjectID, lang, err)
	}
	return nil
}
