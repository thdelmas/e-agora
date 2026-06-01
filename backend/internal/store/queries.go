package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thdelmas/e-agora/backend/internal/model"
	"github.com/thdelmas/e-agora/backend/internal/ranking"
)

var (
	// ErrPoolTooSmall means fewer than two active subjects exist.
	ErrPoolTooSmall = errors.New("store: fewer than two active subjects")
	// ErrInvalidMatchup means a vote referenced ids that aren't both active.
	ErrInvalidMatchup = errors.New("store: invalid matchup")
)

// RandomPair returns two distinct, active subjects (core fields only) for a
// matchup as an **anchor + challenger** pair (docs/05-ranking.md §Matchup
// pairing). Drawing *both* picks by coverage bias surfaced too many pairs of
// mutual unknowns ("voting between two people we don't know about"): a vote only
// carries signal when the visitor can actually judge it. So we draw one of each:
//
//   - anchor — weighted toward fame so the visitor recognizes at least one
//     figure. The weight is cardinality(available_langs): the number of
//     Wikipedia language editions the subject has, a public, non-personal
//     popularity proxy already stored at ingest (no user profiling — privacy
//     intact). Efraimidis–Spirakis top-1 gives P(pick = i) ∝ wᵢ, so famous
//     figures dominate the slot while still rotating across the upper tier. No
//     fixed "is famous" threshold to tune; it self-adjusts to the pool.
//
//   - challenger — the original coverage bias, weight wᵢ = 1/(comparisons+1),
//     drawn over everyone but the anchor. This is the supply side of the
//     Glicko-2 board (it ranks by rating − 2·RD, which buries a subject until
//     its RD shrinks, and RD only shrinks when the subject is shown), so an
//     unproven figure still gets its comparisons — now against a low-RD anchor,
//     which tightens its RD faster per vote than two unknowns meeting would.
//
// Efraimidis–Spirakis weighted sampling without replacement assigns each row a
// key = uⁱ^(1/wᵢ) (u ~ uniform) and takes the largest; P(pick = i) ∝ wᵢ. We
// order by the key in **log space** — ln(key) = ln(u)/wᵢ — instead of computing
// the key directly: power(random(), comparisons+1) underflows to a "value out of
// range" error once a subject has a few hundred comparisons (random()^501 falls
// below the smallest double for ~a quarter of draws), so the naive form is a
// latency bomb for the matchup endpoint as popular subjects rack up votes. We
// use ln(1 - random()) because 1 - random() ∈ (0, 1] is never zero (random() can
// return 0, and ln(0) errors); it has the same distribution as u.
//
//   - anchor:     ln(1-random()) / cardinality(available_langs)  ⇒ wᵢ = cardinality
//   - challenger: (comparisons + 1) * ln(1-random())             ⇒ wᵢ = 1/(comparisons+1)
//
// greatest(card, 1) guards the all-empty-available_langs edge. The final ORDER BY
// random() shuffles which side the anchor lands on, so the familiar figure isn't
// always card A. (Two full scans + sorts; fine for a small pool — see the doc for
// the cached-id-set / TABLESAMPLE path once large.)
//
// Translations are fetched separately per the resolved display language.
func (s *Store) RandomPair(ctx context.Context) ([]model.Subject, error) {
	rows, err := s.pool.Query(ctx, `
		WITH anchor AS (
			SELECT id, wikidata_id, canonical_name, available_langs
			FROM subjects WHERE active
			ORDER BY ln(1 - random()) / greatest(cardinality(available_langs), 1) DESC
			LIMIT 1
		), challenger AS (
			SELECT id, wikidata_id, canonical_name, available_langs
			FROM subjects
			WHERE active AND id NOT IN (SELECT id FROM anchor)
			ORDER BY (comparisons + 1) * ln(1 - random()) DESC
			LIMIT 1
		)
		SELECT id, wikidata_id, canonical_name, available_langs
		FROM (
			SELECT id, wikidata_id, canonical_name, available_langs FROM anchor
			UNION ALL
			SELECT id, wikidata_id, canonical_name, available_langs FROM challenger
		) pair
		ORDER BY random()`)
	if err != nil {
		return nil, fmt.Errorf("random pair: %w", err)
	}
	defer rows.Close()

	var out []model.Subject
	for rows.Next() {
		var s model.Subject
		if err := rows.Scan(&s.ID, &s.WikidataID, &s.CanonicalName, &s.AvailableLangs); err != nil {
			return nil, fmt.Errorf("scan subject: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) < 2 {
		return nil, ErrPoolTooSmall
	}
	return out, nil
}

// Translation returns the cached display content for (subject, lang). found is
// false on a cache miss (the caller may lazily fetch and UpsertTranslation).
func (s *Store) Translation(ctx context.Context, subjectID int64, lang string) (model.Translation, bool, error) {
	var t model.Translation
	err := s.pool.QueryRow(ctx, `
		SELECT subject_id, lang, name, COALESCE(description, ''), COALESCE(extract, ''), COALESCE(image_url, ''), wikipedia_url
		FROM subject_translations WHERE subject_id = $1 AND lang = $2`,
		subjectID, lang,
	).Scan(&t.SubjectID, &t.Lang, &t.Name, &t.Description, &t.Extract, &t.ImageURL, &t.WikipediaURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Translation{}, false, nil
	}
	if err != nil {
		return model.Translation{}, false, fmt.Errorf("translation %d/%s: %w", subjectID, lang, err)
	}
	return t, true, nil
}

// VoteResult reports the contribution count after a vote (for the UI).
type VoteResult struct {
	Contributions int
}

// RecordVote applies one preference atomically: it locks both subjects (in id
// order to avoid deadlocks), updates their Glicko-2 ratings, appends the vote
// (snapshotting pre-vote state), and bumps the session's contribution count
// (docs/05-ranking.md §Applying an update).
func (s *Store) RecordVote(ctx context.Context, sessionID string, winnerID, loserID int64) (VoteResult, error) {
	if winnerID == loserID {
		return VoteResult{}, ErrInvalidMatchup
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return VoteResult{}, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, rating, rd, volatility FROM subjects WHERE id = ANY($1) AND active ORDER BY id FOR UPDATE`,
		[]int64{winnerID, loserID})
	if err != nil {
		return VoteResult{}, fmt.Errorf("lock subjects: %w", err)
	}
	states := map[int64]ranking.Rating{}
	for rows.Next() {
		var id int64
		var rt ranking.Rating
		if err := rows.Scan(&id, &rt.R, &rt.RD, &rt.Vol); err != nil {
			rows.Close()
			return VoteResult{}, err
		}
		states[id] = rt
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return VoteResult{}, err
	}
	if len(states) != 2 {
		return VoteResult{}, ErrInvalidMatchup // one or both inactive/missing
	}

	w, l := states[winnerID], states[loserID]
	nW, nL := ranking.Update(w, l)

	if _, err := tx.Exec(ctx,
		`UPDATE subjects SET rating=$2, rd=$3, volatility=$4, wins=wins+1, comparisons=comparisons+1, updated_at=now() WHERE id=$1`,
		winnerID, nW.R, nW.RD, nW.Vol); err != nil {
		return VoteResult{}, fmt.Errorf("update winner: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE subjects SET rating=$2, rd=$3, volatility=$4, losses=losses+1, comparisons=comparisons+1, updated_at=now() WHERE id=$1`,
		loserID, nL.R, nL.RD, nL.Vol); err != nil {
		return VoteResult{}, fmt.Errorf("update loser: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO votes (session_id, winner_id, loser_id,
			winner_rating_before, loser_rating_before,
			winner_rd_before, loser_rd_before, winner_vol_before, loser_vol_before)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		sessionID, winnerID, loserID, w.R, l.R, w.RD, l.RD, w.Vol, l.Vol); err != nil {
		return VoteResult{}, fmt.Errorf("insert vote: %w", err)
	}

	var contributions int
	if err := tx.QueryRow(ctx,
		`UPDATE sessions SET contributions = contributions + 1 WHERE id = $1 RETURNING contributions`,
		sessionID).Scan(&contributions); err != nil {
		return VoteResult{}, fmt.Errorf("bump session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return VoteResult{}, err
	}
	return VoteResult{Contributions: contributions}, nil
}

// TouchSession upserts the session (minting a row on first sight), refreshes
// last_seen_at, and returns the current row. Anonymous; not authentication.
func (s *Store) TouchSession(ctx context.Context, id string) (model.Session, error) {
	var sess model.Session
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sessions (id) VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET last_seen_at = now()
		RETURNING id, contributions, human_verified_until, created_at, last_seen_at`,
		id,
	).Scan(&sess.ID, &sess.Contributions, &sess.HumanVerifiedUntil, &sess.CreatedAt, &sess.LastSeenAt)
	if err != nil {
		return model.Session{}, fmt.Errorf("touch session: %w", err)
	}
	return sess, nil
}

// MarkHuman sets the human-verified window for a session (R12).
func (s *Store) MarkHuman(ctx context.Context, id string, until time.Time) error {
	if _, err := s.pool.Exec(ctx, `UPDATE sessions SET human_verified_until = $2 WHERE id = $1`, id, until); err != nil {
		return fmt.Errorf("mark human: %w", err)
	}
	return nil
}
