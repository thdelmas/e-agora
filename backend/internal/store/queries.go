package store

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
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

// RecoParams are the recognition-score weights for the matchup draw
// (docs/10-recognition-and-pools.md §2–§3). All public, non-personal —
// derived from Wikipedia pageviews (article facts, never visitor facts).
type RecoParams struct {
	// weight on ln(1+sitelink count) — graceful fallback before pageviews land
	Base float64
	// weight on ln(1+local views) — attention in the visitor's language
	Alpha float64
	// weight on ln(1+global views) — worldwide fame
	Beta float64
	// weight on sphere affinity — the visitor language's share of attention
	Gamma float64
	// flat boost added to a subject in the visitor's home region — a soft
	// bias, not a filter
	Region float64
	// flat boost for a subject in the visitor's home country — a soft bias,
	// finer/stronger than Region
	Country float64
	// probability the challenger is drawn by coverage bias instead of
	// recognition
	DiscoveryRate float64
}

// Pool is the visitor-selected scope for a matchup or leaderboard
// (docs/10-recognition-and-pools.md §4). The axes filter *which* subjects are
// in play; ranking stays one global Glicko rating, so a pool is a view/lens,
// not a separate board. The empty Pool is the whole living pool (the prior
// default).
type Pool struct {
	IncludeDeceased bool   // status axis: include figures who have died
	Continent       string // region axis: "" = any, else e.g. "Europe"
	// region axis (finer): "" = any, else the country's English label, e.g.
	// "France"
	Country string
	FameTop bool // fame-tier axis: restrict to the most-viewed subjects
	// FameTop cutoff as a global_views percentile (e.g. 0.7 = top 30%)
	FamePct float64
}

// RandomPair returns two distinct, active subjects (core fields only) for a
// matchup, drawn to fix the #1 complaint — "two people I've never heard of"
// (docs/10-recognition-and-pools.md). Recognition is *local*, so the draw is
// weighted by a **visitor-relative recognition score** R(s│v) built from
// per-language Wikipedia pageviews, where v is the visitor's language:
//
//	R(s│v) = base·ln(1+langs) + α·ln(1+local) + β·ln(1+global)
//	         + γ·share·ln(1+local)
//
//	  - local  = views of s in language v (0 when s has no article in v)
//	  - global = views of s across all languages (the "recognizable
//	    everywhere" lever)
//	  - share  = local/global (region proxy: a figure read mostly in v belongs
//	    to v's sphere)
//	  - langs  = cardinality(available_langs), a base term that keeps the score
//	    strictly positive and degrades to the old sitelink-count weighting when
//	    no pageview data exists yet (fresh DB, before the first sync).
//
// Both picks are drawn ∝ R, so the visitor recognizes both — via any lever.
// A DiscoveryRate fraction of draws instead pick the challenger by coverage
// bias (wᵢ = 1/(comparisons+1)) so under-compared subjects still accrue the
// votes that shrink their Glicko RD — always against a recognizable anchor,
// never two unknowns. The challenger always honors the pool, so an explicit
// region/fame selection stays strict; cross-pool connectivity (needed for the
// single global rating to stay comparable across pools, docs/10 §4) comes from
// the default pool, whose draws inherently span every continent.
//
// homeRegion / homeCountry are the visitor's home continent and country
// (docs/10 §4) — *soft* biases, distinct from pool.Continent /
// pool.Country's hard filters. A subject in the home region gets a flat
// p.Region added to R, and one in the home country a flat p.Country (finer,
// so weighted stronger), lifting modest *local* figures the visitor knows
// without excluding anyone: the recognition draw leans home, yet the
// discovery challenger (coverage-biased, geo-blind) still reaches across
// continents, so the comparison graph stays connected and the one Glicko
// scale comparable. An empty home value (never chosen, or "whole world") adds
// nothing.
//
// Efraimidis–Spirakis weighted sampling without replacement assigns each row
// a key uᵢ^(1/wᵢ) and takes the largest; P(pick = i) ∝ wᵢ. We order by
// the key in **log space** — ln(key) = ln(1-random())/wᵢ — to avoid the
// underflow that power(random(), …) hits once a weight grows large.
// 1-random() ∈ (0,1] is never zero (ln(0) errors); greatest(w, 1e-9) guards a
// zero weight. The final ORDER BY random() shuffles which side the anchor
// lands on.
//
// Belonging (docs/11) reweights *within* a pool: each subject's recognition
// weight is multiplied by a per-(pool,subject) recall factor
// (n+a)/(π₀·N+a) — 1 when no one has proposed yet (the draw is unchanged
// until data exists), >1 for the crowd-recalled, <1 for a geographic member
// the crowd pointedly doesn't recall here (the Türkiye-in-Europe demotion).
// The discovery slot stays belonging-blind, so a demoted member is rare, not
// deleted, and the comparison graph stays connected.
//
// The pool scopes who is eligible (docs/10 §4): the deceased are excluded
// unless pool.IncludeDeceased, and Continent / Country / FameTop narrow both
// the anchor and the challenger (strict — an explicit selection never leaks
// an out-of-pool figure). Translations are fetched separately per the
// resolved display language.
func (s *Store) RandomPair(
	ctx context.Context, viewerLang, homeRegion, homeCountry string,
	p RecoParams, pool Pool,
) ([]model.Subject, error) {
	discovery := rand.Float64() < p.DiscoveryRate
	// Belonging reweights the draw within this pool (docs/11): a
	// per-(pool,subject) recall factor multiplies the recognition weight,
	// keyed by the pool's scope.
	poolKey := PoolKey(pool)
	// The fame/belonging/membership prior parameters ($10, $17–$22) are CAST
	// ::float8 at every use. They carry fractional values (e.g. memPriorMean
	// 0.8, FamePct 0.7), but a bound parameter with no cast is inferred as
	// integer from the surrounding integer arithmetic — which makes the Beta
	// confidence an integer division and rounds memPriorMean to 0, so the
	// membership gate divides by zero on every draw. Keep the casts.
	rows, err := s.pool.Query(ctx, `
		WITH cutoff AS (
			SELECT CASE WHEN $9 THEN
			            coalesce(percentile_cont($10::float8) `+
		`WITHIN GROUP (ORDER BY global_views), 0)
			       ELSE 0 END AS fame_min
			FROM subjects WHERE active AND ($1 OR died_at IS NULL)
		), belong AS (
			-- N: total proposals into this pool (docs/11); `+
		`0 => factor 1 (no data, no effect).
			SELECT coalesce((SELECT proposals FROM pool_stats `+
		`WHERE pool_key = $16), 0) AS pool_total
		), scored AS (
			SELECT s.id, s.wikidata_id, s.canonical_name, `+
		`s.available_langs, s.died_at, s.comparisons,
			       greatest(
			           (  $4 * ln(1 + greatest(cardinality(s.available_langs), 1))
			            + $5 * ln(1 + coalesce(pv.views, 0))
			            + $6 * ln(1 + s.global_views)
			            + $7 * (coalesce(pv.views, 0)::float8 `+
		`/ greatest(s.global_views, 1)) * ln(1 + coalesce(pv.views, 0))
			            + CASE WHEN $11 <> '' AND s.continent @> ARRAY[$11] `+
		`THEN $12::float8 ELSE 0 END
			            + CASE WHEN $14 <> '' AND s.country @> ARRAY[$14] `+
		`THEN $15::float8 ELSE 0 END )
			         -- belonging factor (docs/11): `+
		`crowd recall reweights within the pool.
			         -- (n+a)/(pi0*N+a): `+
		`1 when no data, >1 recalled, <1 evidence-of-absence.
			         * ((coalesce(pb.proposals, 0) + $17::float8) `+
		`/ ($18::float8 * (SELECT pool_total FROM belong) + $17::float8))
			         -- membership gate (docs/11 §7): confirm/infirm Beta `+
		`confidence, least(1, conf/prior) — suppresses a figure the crowd
			         -- argues out of the pool, never boosts (recall does that).
			         * least(1.0, ((coalesce(pm.confirms, 0) + $19::float8) `+
		`/ (coalesce(pm.confirms, 0) + coalesce(pm.infirms, 0) `+
		`+ $19::float8 + $20::float8)) / $21::float8),
			           1e-9) AS w,
			       (($8 = '' OR s.continent @> ARRAY[$8])
			         AND ($13 = '' OR s.country @> ARRAY[$13])
			         AND s.global_views >= (SELECT fame_min FROM cutoff)
			         -- hard membership drop (docs/11 §7): strong infirm `+
		`consensus removes the figure from the pool entirely.
			         AND ((coalesce(pm.confirms, 0) + $19::float8) `+
		`/ (coalesce(pm.confirms, 0) + coalesce(pm.infirms, 0) `+
		`+ $19::float8 + $20::float8)) >= $22::float8) AS in_pool
			FROM subjects s
			LEFT JOIN subject_pageviews pv `+
		`ON pv.subject_id = s.id AND pv.lang = $3
			LEFT JOIN pool_belonging pb `+
		`ON pb.pool_key = $16 AND pb.subject_id = s.id
			LEFT JOIN pool_membership pm `+
		`ON pm.pool_key = $16 AND pm.subject_id = s.id
			WHERE s.active AND ($1 OR s.died_at IS NULL)
		), anchor AS (
			SELECT id, wikidata_id, canonical_name, available_langs, died_at
			FROM scored WHERE in_pool
			ORDER BY ln(1 - random()) / w DESC
			LIMIT 1
		), challenger AS (
			SELECT id, wikidata_id, canonical_name, available_langs, died_at
			FROM scored
			WHERE id NOT IN (SELECT id FROM anchor) AND in_pool
			ORDER BY CASE WHEN $2 THEN (comparisons + 1) * ln(1 - random())
			              ELSE ln(1 - random()) / w END DESC
			LIMIT 1
		)
		SELECT id, wikidata_id, canonical_name, available_langs, died_at
		FROM (
			SELECT id, wikidata_id, canonical_name, available_langs, `+
		`died_at FROM anchor
			UNION ALL
			SELECT id, wikidata_id, canonical_name, available_langs, `+
		`died_at FROM challenger
		) pair
		ORDER BY random()`,
		pool.IncludeDeceased, discovery, viewerLang, p.Base, p.Alpha,
		p.Beta, p.Gamma,
		pool.Continent, pool.FameTop, pool.FamePct, homeRegion, p.Region,
		pool.Country, homeCountry, p.Country,
		poolKey, belongPriorStrength, belongPriorShare,
		memPriorAlpha, memPriorBeta, memPriorMean, memExcludeBelow)
	if err != nil {
		return nil, fmt.Errorf("random pair: %w", err)
	}
	defer rows.Close()

	var out []model.Subject
	for rows.Next() {
		var s model.Subject
		if err := rows.Scan(
			&s.ID, &s.WikidataID, &s.CanonicalName, &s.AvailableLangs,
			&s.DiedAt,
		); err != nil {
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
func (s *Store) Translation(
	ctx context.Context, subjectID int64, lang string,
) (model.Translation, bool, error) {
	var t model.Translation
	err := s.pool.QueryRow(ctx, `
		SELECT subject_id, lang, name, COALESCE(description, ''), `+
		`COALESCE(extract, ''), COALESCE(image_url, ''), wikipedia_url
		FROM subject_translations WHERE subject_id = $1 AND lang = $2`,
		subjectID, lang,
	).Scan(&t.SubjectID, &t.Lang, &t.Name, &t.Description, &t.Extract,
		&t.ImageURL, &t.WikipediaURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Translation{}, false, nil
	}
	if err != nil {
		return model.Translation{}, false,
			fmt.Errorf("translation %d/%s: %w", subjectID, lang, err)
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
func (s *Store) RecordVote(
	ctx context.Context, sessionID string, winnerID, loserID int64,
) (VoteResult, error) {
	if winnerID == loserID {
		return VoteResult{}, ErrInvalidMatchup
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return VoteResult{}, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, rating, rd, volatility FROM subjects `+
			`WHERE id = ANY($1) AND active ORDER BY id FOR UPDATE`,
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
		`UPDATE subjects SET rating=$2, rd=$3, volatility=$4, wins=wins+1, `+
			`comparisons=comparisons+1, updated_at=now() WHERE id=$1`,
		winnerID, nW.R, nW.RD, nW.Vol); err != nil {
		return VoteResult{}, fmt.Errorf("update winner: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE subjects SET rating=$2, rd=$3, volatility=$4, losses=losses+1, `+
			`comparisons=comparisons+1, updated_at=now() WHERE id=$1`,
		loserID, nL.R, nL.RD, nL.Vol); err != nil {
		return VoteResult{}, fmt.Errorf("update loser: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO votes (session_id, winner_id, loser_id,
			winner_rating_before, loser_rating_before,
			winner_rd_before, loser_rd_before, winner_vol_before, loser_vol_before)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		sessionID, winnerID, loserID, w.R, l.R, w.RD, l.RD, w.Vol,
		l.Vol); err != nil {
		return VoteResult{}, fmt.Errorf("insert vote: %w", err)
	}

	var contributions int
	if err := tx.QueryRow(ctx,
		`UPDATE sessions SET contributions = contributions + 1 `+
			`WHERE id = $1 RETURNING contributions`,
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
func (s *Store) TouchSession(
	ctx context.Context, id string,
) (model.Session, error) {
	var sess model.Session
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sessions (id) VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET last_seen_at = now()
		RETURNING id, contributions, human_verified_until, created_at, last_seen_at`,
		id,
	).Scan(&sess.ID, &sess.Contributions, &sess.HumanVerifiedUntil,
		&sess.CreatedAt, &sess.LastSeenAt)
	if err != nil {
		return model.Session{}, fmt.Errorf("touch session: %w", err)
	}
	return sess, nil
}

// MarkHuman sets the human-verified window for a session (R12).
func (s *Store) MarkHuman(
	ctx context.Context, id string, until time.Time,
) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE sessions SET human_verified_until = $2 WHERE id = $1`,
		id, until); err != nil {
		return fmt.Errorf("mark human: %w", err)
	}
	return nil
}
