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
	AllSubjectQIDs(ctx context.Context) ([]string, error)
	UpsertSubject(
		ctx context.Context, qid, canonicalName, source string,
		langs []string, diedAt string,
	) (int64, error)
	UpsertTranslation(
		ctx context.Context, subjectID int64,
		lang, name, description, extract, imageURL, wikipediaURL string,
	) error
	UpsertPageviews(
		ctx context.Context, subjectID int64, lang string, views int64,
	) error
	RefreshGlobalViews(ctx context.Context) error
	SetSubjectGeo(
		ctx context.Context, qid string, countries, continents []string,
	) error
	SubjectQIDsMissingGeo(ctx context.Context) ([]string, error)
}

// Fetcher is the slice of the upstream clients the seeder depends on.
type Fetcher interface {
	Entity(ctx context.Context, qid string) (EntityFacts, error)
	Summary(ctx context.Context, lang, title string) (Summary, error)
	LeaderQIDs(ctx context.Context) ([]string, error)
	Pageviews(
		ctx context.Context, lang, title string, from, to time.Time,
	) (int64, error)
}

// errSkip marks a candidate as ineligible (not found / not a human / no English
// page) — distinct from a transient failure.
var errSkip = errors.New("skip")

// seedItem is one entry of un_leaders.json / seed_extra.json.
type seedItem struct {
	QID  string `json:"qid"`
	Name string `json:"name"`
}

// PageviewOpts configures the recognition-signal pass (docs/10): which language
// editions to record pageviews for, and the trailing window to sum. An empty
// Langs (or non-positive Window) disables it — the draw then falls back to
// sitelink-count weighting.
type PageviewOpts struct {
	Langs  []string
	Window time.Duration
}

// Seeder enriches snapshot QIDs from Wikidata/Wikipedia and upserts them.
type Seeder struct {
	Fetcher Fetcher
	Store   SubjectWriter
	Logger  *slog.Logger
	Delay   time.Duration // politeness pause between upstream calls
	// PageviewLangs are the served languages to record pageviews for
	// (empty = skip).
	PageviewLangs  []string
	PageviewWindow time.Duration // trailing window summed per language

	// countryCache maps a P27 QID → resolved (label, continent), one fetch per
	// country per pass.
	countryCache map[string]countryInfo
}

// Run builds a production seeder (live clients) and seeds honoring mode. main
// calls this, typically in a background goroutine.
func Run(
	ctx context.Context, w SubjectWriter, mode string, pv PageviewOpts,
	logger *slog.Logger,
) error {
	return (&Seeder{
		Fetcher:        NewClient(),
		Store:          w,
		Logger:         logger,
		Delay:          150 * time.Millisecond,
		PageviewLangs:  pv.Langs,
		PageviewWindow: pv.Window,
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
			s.Logger.Info("seed: pool already populated, skipping",
				"subjects", n)
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
			s.Logger.Warn("seed: cancelled", "completed", i,
				"candidates", len(items))
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
	s.Logger.Info("seed: done", "upserted", upserted, "skipped", skipped,
		"failed", failed, "candidates", len(items))
	s.refreshGlobalViews(ctx)
	return nil
}

// ScheduleSync runs SyncOnce on a fixed cadence until ctx is cancelled (it is a
// blocking loop — run it in a goroutine). interval ≤ 0 disables it (returns
// immediately). The first sync fires after one interval, not at boot: startup
// seeding already covers a fresh deploy, and re-fetching the whole pool on
// every (possibly frequent) cold-start would be wasteful. NOTE: this is an
// in-process timer — on a host that sleeps when idle (e.g. Render free) it
// only fires while the instance is awake; an always-on host gets a true daily
// refresh.
func ScheduleSync(
	ctx context.Context, w SubjectWriter, interval time.Duration,
	pv PageviewOpts, logger *slog.Logger,
) {
	if interval <= 0 {
		logger.Info("sync: disabled", "interval", interval.String())
		return
	}
	logger.Info("sync: scheduled", "interval", interval.String())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := Sync(ctx, w, pv, logger)
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("sync failed", "err", err)
			}
		}
	}
}

// Sync runs a single refresh pass with production clients. main schedules it
// via ScheduleSync; it's also the unit a future admin trigger would call.
func Sync(
	ctx context.Context, w SubjectWriter, pv PageviewOpts,
	logger *slog.Logger,
) error {
	return (&Seeder{
		Fetcher:        NewClient(),
		Store:          w,
		Logger:         logger,
		Delay:          150 * time.Millisecond,
		PageviewLangs:  pv.Langs,
		PageviewWindow: pv.Window,
	}).SyncOnce(ctx)
}

// refreshGlobalViews materializes subjects.global_views from the per-language
// counts after a pass. Best-effort and a no-op when the pageview pass is
// disabled (nothing new to sum). A failure is logged, not fatal — stale
// global_views still yields a usable draw.
func (s *Seeder) refreshGlobalViews(ctx context.Context) {
	if len(s.PageviewLangs) == 0 || s.PageviewWindow <= 0 {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	if err := s.Store.RefreshGlobalViews(ctx); err != nil {
		s.Logger.Warn("sync: refresh global_views failed", "err", err)
	}
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

	id, err := s.Store.UpsertSubject(
		ctx, it.QID, name, "seed", langs, facts.DiedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert subject: %w", err)
	}

	// Region pool axis (docs/10 §4): resolve the subject's citizenships (P27) to
	// country labels + continents, best-effort (a failure leaves geo NULL).
	if len(facts.CountryQIDs) > 0 {
		countries, continents := s.resolveCountries(ctx, facts.CountryQIDs)
		if len(countries) > 0 || len(continents) > 0 {
			if err := s.Store.SetSubjectGeo(
				ctx, it.QID, countries, continents,
			); err != nil {
				s.Logger.Warn("seed: set geo failed", "qid", it.QID,
					"err", err)
			}
		}
	}

	// English translation — the universal R9 fallback content.
	enURL := "https://en.wikipedia.org/wiki/" +
		strings.ReplaceAll(facts.EnwikiTitle, " ", "_")
	if sum, err := s.Fetcher.Summary(ctx, "en", facts.EnwikiTitle); err != nil {
		// Degraded: the sitelink title still yields a real page (R2 satisfied);
		// description/image fill in on a later EAGORA_SEED=force.
		s.Logger.Warn("seed: en summary failed, degraded translation",
			"qid", it.QID, "err", err)
		if err := s.Store.UpsertTranslation(
			ctx, id, "en", name, "", "", "", enURL,
		); err != nil {
			return err
		}
	} else {
		url := sum.WikipediaURL
		if url == "" {
			url = enURL
		}
		if err := s.Store.UpsertTranslation(
			ctx, id, "en", firstNonEmpty(sum.Name, name),
			sum.Description, sum.Extract, sum.ImageURL, url,
		); err != nil {
			return err
		}
	}

	// Recognition signal (docs/10): record trailing-window pageviews per served
	// language using the sitelink titles already on facts (no extra title fetch).
	s.ingestPageviews(ctx, id, facts)
	return nil
}

// ingestPageviews records each served language's trailing-window view count for
// one subject. Best-effort: a per-language fetch/upsert failure is logged and
// skipped (a missing count weighs as zero, not a seed failure). A no-op when
// the pass is disabled or the subject has no sitelink in a served language.
func (s *Seeder) ingestPageviews(
	ctx context.Context, id int64, facts EntityFacts,
) {
	if len(s.PageviewLangs) == 0 || s.PageviewWindow <= 0 {
		return
	}
	to := time.Now().UTC()
	from := to.Add(-s.PageviewWindow)
	for _, lang := range s.PageviewLangs {
		title := facts.Sitelinks[lang]
		if title == "" {
			continue // no article in this language — nothing to count
		}
		if ctx.Err() != nil {
			return
		}
		views, err := s.Fetcher.Pageviews(ctx, lang, title, from, to)
		if err != nil {
			s.Logger.Warn("seed: pageviews fetch failed", "qid", facts.QID,
				"lang", lang, "err", err)
			continue
		}
		if err := s.Store.UpsertPageviews(ctx, id, lang, views); err != nil {
			s.Logger.Warn("seed: pageviews upsert failed", "qid", facts.QID,
				"lang", lang, "err", err)
		}
		if s.Delay > 0 {
			select {
			case <-time.After(s.Delay):
			case <-ctx.Done():
				return
			}
		}
	}
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
