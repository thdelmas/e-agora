package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Membership-confidence constants (docs/11 §7). Confirm/infirm votes on a shown
// geographic membership feed a Beta posterior: a subject "belongs" with
// probability (confirms + α) / (confirms + infirms + α + β). The prior is
// deliberately trusting — Wikidata P27 placed the figure here, so with no human
// votes confidence sits at the prior mean α/(α+β) = 0.8, not 0.5. It takes a
// sustained run of infirms to argue a figure out; a few confirms defend one.
const (
	// α — prior "belongs" weight (trust the P27 seed)
	memPriorAlpha = 4.0
	// β — prior "doesn't belong" weight
	memPriorBeta = 1.0
	// prior mean α/(α+β) = 0.8
	memPriorMean = memPriorAlpha / (memPriorAlpha + memPriorBeta)
	// hard-drop from the pool below this confidence
	memExcludeBelow = 0.25
)

// MembershipConfidence is the Beta posterior mean that a subject belongs in a
// pool given its confirm/infirm tally (docs/11 §7): (c+α)/(c+i+α+β). Empty
// tally returns the prior mean (0.8 — trust the geographic seed). Confidence
// falls toward 0 as infirms pile up and recovers toward 1 with confirms.
func MembershipConfidence(confirms, infirms int) float64 {
	return (float64(confirms) + memPriorAlpha) /
		(float64(confirms+infirms) + memPriorAlpha + memPriorBeta)
}

// MembershipGate is the draw multiplier the confidence implies: it never boosts
// a figure (the geographic seed already includes them, and recall is the
// booster) but suppresses one the crowd is arguing out — 1 at or above the
// trusting prior, sliding toward 0 as confidence drops. least(1, conf/π_m).
func MembershipGate(confirms, infirms int) float64 {
	g := MembershipConfidence(confirms, infirms) / memPriorMean
	if g > 1 {
		return 1
	}
	return g
}

// MembershipExcluded reports whether the crowd has argued a figure out of a
// pool hard enough to drop them from it entirely (leaderboard + draw), versus
// merely down-weighting the draw. The threshold is conservative: overriding
// Wikidata takes real consensus, not one or two dissents (docs/11 §7).
func MembershipExcluded(confirms, infirms int) bool {
	return MembershipConfidence(confirms, infirms) < memExcludeBelow
}

// MembershipStat is a subject's membership tally in one pool plus the calling
// session's own standing verdict (so the UI can show which button is active).
// Verdict: +1 confirm, -1 infirm, 0 none.
type MembershipStat struct {
	Confirms int     `json:"confirms"`
	Infirms  int     `json:"infirms"`
	Score    float64 `json:"score"`   // MembershipConfidence(Confirms, Infirms)
	Verdict  int     `json:"verdict"` // this session's vote
}

// ErrMembershipScope rejects a membership vote on a non-geographic pool: the
// 'world' pool has no membership question — everyone belongs to the world.
var ErrMembershipScope = errors.New("store: membership needs a region pool")

// RecordMembershipVote sets the calling session's standing verdict on a
// subject's membership in a pool and returns the refreshed tally (docs/11 §7).
// verdict +1/-1 sets confirm/infirm; 0 retracts. The session's vote is upserted
// (one standing verdict per session/pool/subject, changeable) and the
// pool_membership aggregate is recomputed from the votes in the same
// transaction — recount, not delta, so a flipped or retracted vote stays exact.
// The subject must be active and the pool must be a region pool.
func (s *Store) RecordMembershipVote(
	ctx context.Context, sessionID, poolKey string, subjectID int64, verdict int,
) (MembershipStat, error) {
	if poolKey == "" || poolKey == "world" {
		return MembershipStat{}, ErrMembershipScope
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MembershipStat{}, err
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	var active bool
	err = tx.QueryRow(ctx,
		`SELECT active FROM subjects WHERE id = $1`, subjectID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return MembershipStat{}, ErrInvalidProposal
	}
	if err != nil {
		return MembershipStat{}, fmt.Errorf("membership: check subject: %w", err)
	}

	if verdict == 0 {
		if _, err := tx.Exec(ctx,
			`DELETE FROM membership_votes
			 WHERE session_id = $1 AND pool_key = $2 AND subject_id = $3`,
			sessionID, poolKey, subjectID); err != nil {
			return MembershipStat{}, fmt.Errorf("membership: retract: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx,
			`INSERT INTO membership_votes `+
				`(session_id, pool_key, subject_id, verdict)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (session_id, pool_key, subject_id)
			 DO UPDATE SET verdict = EXCLUDED.verdict, updated_at = now()`,
			sessionID, poolKey, subjectID, verdict); err != nil {
			return MembershipStat{}, fmt.Errorf("membership: upsert vote: %w", err)
		}
	}

	// Recompute the tally for this (pool, subject) from the source-of-truth votes
	// and materialize it; drop the aggregate row once no votes remain.
	var confirms, infirms int
	if err := tx.QueryRow(ctx,
		`SELECT
		   coalesce(sum(CASE WHEN verdict =  1 THEN 1 END), 0),
		   coalesce(sum(CASE WHEN verdict = -1 THEN 1 END), 0)
		 FROM membership_votes WHERE pool_key = $1 AND subject_id = $2`,
		poolKey, subjectID).Scan(&confirms, &infirms); err != nil {
		return MembershipStat{}, fmt.Errorf("membership: recount: %w", err)
	}
	if confirms == 0 && infirms == 0 {
		if _, err := tx.Exec(ctx,
			`DELETE FROM pool_membership WHERE pool_key = $1 AND subject_id = $2`,
			poolKey, subjectID); err != nil {
			return MembershipStat{}, fmt.Errorf("membership: clear agg: %w", err)
		}
	} else if _, err := tx.Exec(ctx,
		`INSERT INTO pool_membership (pool_key, subject_id, confirms, infirms)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (pool_key, subject_id)
		 DO UPDATE SET confirms = EXCLUDED.confirms, infirms = EXCLUDED.infirms,
		               updated_at = now()`,
		poolKey, subjectID, confirms, infirms); err != nil {
		return MembershipStat{}, fmt.Errorf("membership: write agg: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return MembershipStat{}, fmt.Errorf("membership: commit: %w", err)
	}
	return MembershipStat{
		Confirms: confirms, Infirms: infirms,
		Score:   MembershipConfidence(confirms, infirms),
		Verdict: verdict,
	}, nil
}

// SubjectGeo returns a subject's resolved citizenships and continents (the
// region-pool axes, docs/10 §4) — the basis for the per-subject "which pools is
// this figure in, and why" list. found is false when no subject has that id.
func (s *Store) SubjectGeo(
	ctx context.Context, id int64,
) (countries, continents []string, found bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT coalesce(country, '{}'), coalesce(continent, '{}')
		 FROM subjects WHERE id = $1`, id).Scan(&countries, &continents)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("subject geo: %w", err)
	}
	return countries, continents, true, nil
}

// MembershipFor returns each subject's membership tally in the pool plus the
// session's own verdict, for the requested ids (matchup pair or leaderboard
// page). Subjects with no votes come back at the prior (zero tally, prior-mean
// score, verdict 0). One query: unnest the ids and left-join the aggregate and
// the session's standing votes.
func (s *Store) MembershipFor(
	ctx context.Context, poolKey, sessionID string, subjectIDs []int64,
) (map[int64]MembershipStat, error) {
	out := make(map[int64]MembershipStat, len(subjectIDs))
	if poolKey == "" || poolKey == "world" || len(subjectIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT sid,
		       coalesce(pm.confirms, 0), coalesce(pm.infirms, 0),
		       coalesce(mv.verdict, 0)
		FROM unnest($3::bigint[]) AS sid
		LEFT JOIN pool_membership pm
		       ON pm.pool_key = $1 AND pm.subject_id = sid
		LEFT JOIN membership_votes mv
		       ON mv.pool_key = $1 AND mv.subject_id = sid
		      AND mv.session_id = $2`,
		poolKey, sessionID, subjectIDs)
	if err != nil {
		return nil, fmt.Errorf("membership for: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var m MembershipStat
		if err := rows.Scan(&id, &m.Confirms, &m.Infirms, &m.Verdict); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		m.Score = MembershipConfidence(m.Confirms, m.Infirms)
		out[id] = m
	}
	return out, rows.Err()
}
