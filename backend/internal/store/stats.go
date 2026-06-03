package store

import (
	"context"
	"fmt"

	"github.com/thdelmas/e-agora/backend/internal/model"
)

// Stats returns an aggregate, privacy-preserving activity snapshot: all-time
// headline totals plus a gap-filled daily time series for the trailing `days`
// UTC-days (docs/04-api.md §GET /api/stats). Everything is a COUNT over
// anonymous data — no per-visitor rows leave the database, and the visitor's
// IP is never stored to begin with, so nothing here can identify anyone.
//
// "Visitors" is derived from sessions.created_at — the first time a browser
// is seen — so it counts *new* anonymous browsers per day rather than raw
// page views (which e-agora deliberately does not log). At v1 scale the daily
// aggregation full-scans votes/sessions/subjects; revisit with a rollup table
// if the pool grows large.
func (s *Store) Stats(ctx context.Context, days int) (model.Stats, error) {
	var st model.Stats

	// All-time totals in a single round trip.
	if err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM votes),
			(SELECT count(DISTINCT session_id) FROM votes),
			(SELECT count(*) FROM sessions),
			(SELECT count(*) FROM subjects WHERE active),
			(SELECT count(*) FROM subjects WHERE source = 'user')`,
	).Scan(
		&st.Totals.Votes, &st.Totals.Voters, &st.Totals.Visitors,
		&st.Totals.Subjects, &st.Totals.UserContributed,
	); err != nil {
		return model.Stats{}, fmt.Errorf("stats totals: %w", err)
	}

	// Daily series, bucketed by UTC calendar day. generate_series gap-fills the
	// window so the client always gets exactly `days` contiguous points (zeros
	// where there was no activity). created_at is timestamptz; AT TIME ZONE 'UTC'
	// pins the bucket boundary to UTC regardless of the server's zone.
	rows, err := s.pool.Query(ctx, `
		WITH days AS (
			SELECT generate_series(
				date_trunc('day', now() AT TIME ZONE 'UTC') `+
		`- (($1::int - 1) * interval '1 day'),
				date_trunc('day', now() AT TIME ZONE 'UTC'),
				interval '1 day'
			)::date AS day
		),
		v AS (
			SELECT (created_at AT TIME ZONE 'UTC')::date AS day,
			       count(*) AS votes, count(DISTINCT session_id) AS voters
			FROM votes GROUP BY 1
		),
		ses AS (
			SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS visitors
			FROM sessions GROUP BY 1
		),
		adds AS (
			SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS added
			FROM subjects WHERE source = 'user' GROUP BY 1
		)
		SELECT d.day,
		       COALESCE(v.votes, 0), COALESCE(v.voters, 0),
		       COALESCE(ses.visitors, 0), COALESCE(adds.added, 0)
		FROM days d
		LEFT JOIN v    ON v.day    = d.day
		LEFT JOIN ses  ON ses.day  = d.day
		LEFT JOIN adds ON adds.day = d.day
		ORDER BY d.day`, days)
	if err != nil {
		return model.Stats{}, fmt.Errorf("stats daily: %w", err)
	}
	defer rows.Close()

	st.Daily = make([]model.DailyStat, 0, days)
	for rows.Next() {
		var d model.DailyStat
		if err := rows.Scan(&d.Date, &d.Votes, &d.Voters, &d.Visitors,
			&d.Added); err != nil {
			return model.Stats{}, fmt.Errorf("scan daily stat: %w", err)
		}
		st.Daily = append(st.Daily, d)
	}
	if err := rows.Err(); err != nil {
		return model.Stats{}, err
	}
	return st, nil
}
