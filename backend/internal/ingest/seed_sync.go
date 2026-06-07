package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// BackfillGeo builds a production seeder and resolves region-pool geo
// (country/continent) for any subject still missing it. main runs it once at
// startup, after seeding, so a deploy that adds the pools feature to an
// existing pool populates the region pools immediately instead of waiting on
// the daily sync (docs/10 §4). Cheap on a healthy pool: a fully-resolved DB
// yields an empty work-list and zero upstream calls.
func BackfillGeo(
	ctx context.Context, w SubjectWriter, logger *slog.Logger,
) error {
	return (&Seeder{
		Fetcher: NewClient(),
		Store:   w,
		Logger:  logger,
		Delay:   150 * time.Millisecond,
	}).BackfillGeo(ctx)
}

// BackfillGeo resolves and stores country/continent for subjects whose
// continent is still NULL (docs/10 §4) — the figures that predate the pools
// feature, for which seedOne never ran. It re-fetches only those subjects (one
// Wikidata entity each, with the country lookup cached per pass), so it's far
// lighter than a full SyncOnce and self-heals existing deployments without a
// manual re-seed. The per-subject failure mode matches seeding: a transient
// error is logged and the pass continues; a subject with no resolvable country
// simply stays unscoped.
func (s *Seeder) BackfillGeo(ctx context.Context) error {
	qids, err := s.Store.SubjectQIDsMissingGeo(ctx)
	if err != nil {
		return fmt.Errorf("backfill geo: list subjects: %w", err)
	}
	if len(qids) == 0 {
		s.Logger.Info("backfill geo: nothing to resolve")
		return nil
	}
	s.Logger.Info("backfill geo: starting", "subjects", len(qids))

	var resolved, unscoped, failed int
	for i, qid := range qids {
		if err := ctx.Err(); err != nil {
			s.Logger.Warn("backfill geo: cancelled", "completed", i,
				"subjects", len(qids))
			return err
		}
		facts, err := s.Fetcher.Entity(ctx, qid)
		if err != nil {
			failed++
			s.Logger.Warn("backfill geo: entity failed", "qid", qid, "err", err)
			continue
		}
		var countries, continents []string
		if len(facts.CountryQIDs) > 0 {
			countries, continents = s.resolveCountries(ctx, facts.CountryQIDs)
		}
		if len(countries) == 0 && len(continents) == 0 {
			unscoped++ // no resolvable country/continent — leave NULL
			continue
		}
		if err := s.Store.SetSubjectGeo(
			ctx, qid, countries, continents,
		); err != nil {
			failed++
			s.Logger.Warn("backfill geo: set geo failed", "qid", qid,
				"err", err)
			continue
		}
		resolved++
		if s.Delay > 0 {
			select {
			case <-time.After(s.Delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	s.Logger.Info("backfill geo: done", "resolved", resolved,
		"unscoped", unscoped, "failed", failed)
	return nil
}

// SyncOnce re-ingests the whole pool from Wikidata/Wikipedia and discovers
// newly-elected leaders (docs/06-wikipedia-ingestion.md §Step 5):
//   - refresh: every subject already in the DB (seed and user-added) is
//     re-fetched and upserted, so metadata — name, available languages, and
//     date of death (P570, the deceased filter's signal) — tracks Wikidata
//     over time. The upsert preserves ratings and vote history.
//   - discover: the live UN head-of-state/government SPARQL query (§Step 1)
//     adds any sitting leader not yet in the pool. Best-effort — a WDQS
//     failure logs a warning and the refresh still runs.
//
// Both halves funnel through the same per-candidate path as seeding (seedOne),
// so eligibility (human + English page) and translation caching behave
// identically.
func (s *Seeder) SyncOnce(ctx context.Context) error {
	existing, err := s.Store.AllSubjectQIDs(ctx)
	if err != nil {
		return fmt.Errorf("sync: list subjects: %w", err)
	}

	seen := make(map[string]bool, len(existing))
	candidates := make([]seedItem, 0, len(existing))
	for _, qid := range existing {
		if qid == "" || seen[qid] {
			continue
		}
		seen[qid] = true
		candidates = append(candidates, seedItem{QID: qid})
	}
	// everything before this index is a known subject
	refreshCount := len(candidates)

	discovered := 0
	if leaders, err := s.Fetcher.LeaderQIDs(ctx); err != nil {
		s.Logger.Warn(
			"sync: leader discovery failed, refreshing existing only",
			"err", err)
	} else {
		for _, qid := range leaders {
			if qid == "" || seen[qid] {
				continue
			}
			seen[qid] = true
			candidates = append(candidates, seedItem{QID: qid})
			discovered++
		}
	}
	s.Logger.Info("sync: starting", "existing", refreshCount,
		"newly_discovered", discovered)

	var refreshed, added, skipped, failed int
	for i, it := range candidates {
		if err := ctx.Err(); err != nil {
			s.Logger.Warn("sync: cancelled", "completed", i,
				"candidates", len(candidates))
			return err
		}
		isNew := i >= refreshCount
		switch err := s.seedOne(ctx, it); {
		case err == nil:
			if isNew {
				added++
			} else {
				refreshed++
			}
		case errors.Is(err, errSkip):
			skipped++
		default:
			failed++
			s.Logger.Warn("sync: subject failed", "qid", it.QID, "err", err)
		}
		if s.Delay > 0 {
			select {
			case <-time.After(s.Delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	s.Logger.Info("sync: done", "refreshed", refreshed, "added", added,
		"skipped", skipped, "failed", failed)
	s.refreshGlobalViews(ctx)
	return nil
}
