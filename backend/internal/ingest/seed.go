package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/thdelmas/e-agora/backend/data"
)

// SubjectWriter is the slice of the store the seeder depends on (defined here,
// at the consumer, so the seeder is testable with a fake).
type SubjectWriter interface {
	CountSubjects(ctx context.Context) (int, error)
	UpsertSubject(ctx context.Context, qid, canonicalName, source string, langs []string) (int64, error)
	UpsertTranslation(ctx context.Context, subjectID int64, lang, name, description, imageURL, wikipediaURL string) error
}

// Fetcher is the slice of the upstream clients the seeder depends on.
type Fetcher interface {
	Entity(ctx context.Context, qid string) (EntityFacts, error)
	Summary(ctx context.Context, lang, title string) (Summary, error)
}

// errSkip marks a candidate as ineligible (not found / not a human / no English
// page) — distinct from a transient failure.
var errSkip = errors.New("skip")

// seedItem is one entry of un_leaders.json / seed_extra.json.
type seedItem struct {
	QID  string `json:"qid"`
	Name string `json:"name"`
}

// Seeder enriches snapshot QIDs from Wikidata/Wikipedia and upserts them.
type Seeder struct {
	Fetcher Fetcher
	Store   SubjectWriter
	Logger  *slog.Logger
	Delay   time.Duration // politeness pause between subjects
}

// Run builds a production seeder (live clients) and seeds honoring mode. main
// calls this, typically in a background goroutine.
func Run(ctx context.Context, w SubjectWriter, mode string, logger *slog.Logger) error {
	return (&Seeder{
		Fetcher: NewClient(),
		Store:   w,
		Logger:  logger,
		Delay:   150 * time.Millisecond,
	}).Seed(ctx, mode)
}

// Seed runs the seed-on-startup step (docs/06-wikipedia-ingestion.md §Step 3):
//   - off:   never contact upstreams.
//   - auto:  seed only if the pool is empty (default).
//   - force: re-ingest all (upserts; ratings/votes preserved).
func (s *Seeder) Seed(ctx context.Context, mode string) error {
	switch mode {
	case "off":
		s.Logger.Info("seed: disabled", "mode", mode)
		return nil
	case "auto":
		n, err := s.Store.CountSubjects(ctx)
		if err != nil {
			return fmt.Errorf("seed: count subjects: %w", err)
		}
		if n > 0 {
			s.Logger.Info("seed: pool already populated, skipping", "subjects", n)
			return nil
		}
	case "force":
		// Re-ingest everything; upserts preserve ratings and vote history.
	default:
		return fmt.Errorf("seed: unknown EAGORA_SEED mode %q", mode)
	}

	items, err := loadSeedItems()
	if err != nil {
		return fmt.Errorf("seed: load snapshot: %w", err)
	}
	s.Logger.Info("seed: starting", "mode", mode, "candidates", len(items))

	var upserted, skipped, failed int
	for i, it := range items {
		if err := ctx.Err(); err != nil {
			s.Logger.Warn("seed: cancelled", "completed", i, "candidates", len(items))
			return err
		}
		switch err := s.seedOne(ctx, it); {
		case err == nil:
			upserted++
		case errors.Is(err, errSkip):
			skipped++
		default:
			// Transient/network error: log and continue — a partial pool is valid.
			failed++
			s.Logger.Warn("seed: subject failed", "qid", it.QID, "err", err)
		}
		if s.Delay > 0 {
			select {
			case <-time.After(s.Delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	s.Logger.Info("seed: done", "upserted", upserted, "skipped", skipped, "failed", failed, "candidates", len(items))
	return nil
}

// seedOne enriches and upserts a single candidate. Returns errSkip for
// ineligible candidates, a wrapped error for transient failures.
func (s *Seeder) seedOne(ctx context.Context, it seedItem) error {
	facts, err := s.Fetcher.Entity(ctx, it.QID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errSkip
		}
		return fmt.Errorf("entity: %w", err)
	}
	if !facts.IsHuman { // R8: people only
		return errSkip
	}
	if facts.EnwikiTitle == "" { // R9 fallback needs an English page
		return errSkip
	}

	name := firstNonEmpty(facts.LabelEn, it.Name, facts.EnwikiTitle)
	langs := facts.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}

	id, err := s.Store.UpsertSubject(ctx, it.QID, name, "seed", langs)
	if err != nil {
		return fmt.Errorf("upsert subject: %w", err)
	}

	// English translation — the universal R9 fallback content.
	enURL := "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(facts.EnwikiTitle, " ", "_")
	sum, err := s.Fetcher.Summary(ctx, "en", facts.EnwikiTitle)
	if err != nil {
		// Degraded: the sitelink title still yields a real page (R2 satisfied);
		// description/image fill in on a later EAGORA_SEED=force.
		s.Logger.Warn("seed: en summary failed, degraded translation", "qid", it.QID, "err", err)
		return s.Store.UpsertTranslation(ctx, id, "en", name, "", "", enURL)
	}
	url := sum.WikipediaURL
	if url == "" {
		url = enURL
	}
	return s.Store.UpsertTranslation(ctx, id, "en", firstNonEmpty(sum.Name, name), sum.Description, sum.ImageURL, url)
}

// loadSeedItems reads and dedupes (by QID) the embedded snapshots.
func loadSeedItems() ([]seedItem, error) {
	var all []seedItem
	for _, name := range []string{"un_leaders.json", "seed_extra.json"} {
		b, err := data.FS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var items []seedItem
		if err := json.Unmarshal(b, &items); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		all = append(all, items...)
	}

	seen := make(map[string]bool, len(all))
	out := make([]seedItem, 0, len(all))
	for _, it := range all {
		if it.QID == "" || seen[it.QID] {
			continue
		}
		seen[it.QID] = true
		out = append(out, it)
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
