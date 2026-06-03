package ingest

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSeed_OffMode(t *testing.T) {
	w := &fakeWriter{}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{}}
	if err := s.Seed(context.Background(), "off"); err != nil {
		t.Fatalf("off: %v", err)
	}
	if w.subjects != 0 {
		t.Errorf("off mode wrote %d subjects, want 0", w.subjects)
	}
}

func TestSeed_AutoSkipsWhenPopulated(t *testing.T) {
	w := &fakeWriter{count: 42}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{}}
	if err := s.Seed(context.Background(), "auto"); err != nil {
		t.Fatalf("auto: %v", err)
	}
	if w.subjects != 0 {
		t.Errorf("auto mode seeded a populated pool (%d)", w.subjects)
	}
}

func TestSeed_UnknownMode(t *testing.T) {
	s := &Seeder{
		Store: &fakeWriter{}, Logger: discardLogger(), Fetcher: fakeFetcher{},
	}
	if err := s.Seed(context.Background(), "nonsense"); err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestSeedOne(t *testing.T) {
	humanFacts := EntityFacts{
		QID: "Q1", IsHuman: true, LabelEn: "A Leader",
		EnwikiTitle: "A_Leader", Langs: []string{"en", "fr"},
	}
	okSummary := Summary{
		Name: "A Leader", Description: "A politician.", ImageURL: "i",
		WikipediaURL: "u", Type: "standard",
	}

	cases := []struct {
		name      string
		fetcher   fakeFetcher
		wantErr   error // nil, errSkip, or sentinel "any"
		wantSubj  int
		wantTrans int
	}{
		{
			name: "human upserts subject + translation",
			fetcher: fakeFetcher{
				entity:  func(string) (EntityFacts, error) { return humanFacts, nil },
				summary: func(string, string) (Summary, error) { return okSummary, nil },
			},
			wantErr: nil, wantSubj: 1, wantTrans: 1,
		},
		{
			name: "not a human is skipped",
			fetcher: fakeFetcher{
				entity: func(string) (EntityFacts, error) {
					return EntityFacts{QID: "Q1", IsHuman: false, EnwikiTitle: "X"}, nil
				},
			},
			wantErr: errSkip, wantSubj: 0, wantTrans: 0,
		},
		{
			name: "no English page is skipped",
			fetcher: fakeFetcher{
				entity: func(string) (EntityFacts, error) {
					return EntityFacts{QID: "Q1", IsHuman: true, EnwikiTitle: ""}, nil
				},
			},
			wantErr: errSkip, wantSubj: 0, wantTrans: 0,
		},
		{
			name: "not found is skipped",
			fetcher: fakeFetcher{
				entity: func(string) (EntityFacts, error) {
					return EntityFacts{}, ErrNotFound
				},
			},
			wantErr: errSkip, wantSubj: 0, wantTrans: 0,
		},
		{
			name: "summary failure still stores a degraded translation",
			fetcher: fakeFetcher{
				entity: func(string) (EntityFacts, error) {
					return humanFacts, nil
				},
				summary: func(string, string) (Summary, error) {
					return Summary{}, errors.New("network")
				},
			},
			wantErr: nil, wantSubj: 1, wantTrans: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := &fakeWriter{}
			s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: c.fetcher}
			err := s.seedOne(context.Background(), seedItem{QID: "Q1"})
			if c.wantErr == errSkip && !errors.Is(err, errSkip) {
				t.Errorf("err = %v, want errSkip", err)
			}
			if c.wantErr == nil && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if w.subjects != c.wantSubj {
				t.Errorf("subjects = %d, want %d", w.subjects, c.wantSubj)
			}
			if len(w.translations) != c.wantTrans {
				t.Errorf("translations = %d, want %d",
					len(w.translations), c.wantTrans)
			}
		})
	}
}

func TestSeedOne_DegradedURLIsRealPage(t *testing.T) {
	w := &fakeWriter{}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(string) (EntityFacts, error) {
			return EntityFacts{
				QID: "Q1", IsHuman: true, EnwikiTitle: "Angela Merkel",
				Langs: []string{"en"},
			}, nil
		},
		summary: func(string, string) (Summary, error) {
			return Summary{}, errors.New("offline")
		},
	}}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	wantURL := "https://en.wikipedia.org/wiki/Angela_Merkel"
	if len(w.translations) != 1 || w.translations[0].url != wantURL {
		t.Errorf("degraded url = %+v", w.translations)
	}
}

func TestSyncOnce_RefreshesExistingAndDiscoversNew(t *testing.T) {
	// Two subjects already in the pool; discovery returns one duplicate (Q2) and
	// one genuinely new leader (Q3). Expect all three upserted: Q1+Q2 refreshed,
	// Q3 added.
	w := &fakeWriter{qids: []string{"Q1", "Q2"}}
	human := func(qid string) (EntityFacts, error) {
		return EntityFacts{
			QID: qid, IsHuman: true, LabelEn: qid, EnwikiTitle: qid,
			Langs: []string{"en"},
		}, nil
	}
	s := &Seeder{
		Store:  w,
		Logger: discardLogger(),
		Fetcher: fakeFetcher{
			entity: human,
			summary: func(string, string) (Summary, error) {
				return Summary{Name: "n", WikipediaURL: "u"}, nil
			},
			leaders: func() ([]string, error) {
				return []string{"Q2", "Q3"}, nil
			},
		},
	}
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if w.subjects != 3 {
		t.Errorf("upserts = %d, want 3 (Q1,Q2 refreshed + Q3 discovered)",
			w.subjects)
	}
}

func TestSyncOnce_DiscoveryFailureStillRefreshes(t *testing.T) {
	// A WDQS outage must not block the refresh of the existing pool.
	w := &fakeWriter{qids: []string{"Q1"}}
	s := &Seeder{
		Store:  w,
		Logger: discardLogger(),
		Fetcher: fakeFetcher{
			entity: func(qid string) (EntityFacts, error) {
				return EntityFacts{
					QID: qid, IsHuman: true, LabelEn: qid, EnwikiTitle: qid,
					Langs: []string{"en"},
				}, nil
			},
			summary: func(string, string) (Summary, error) {
				return Summary{Name: "n", WikipediaURL: "u"}, nil
			},
			leaders: func() ([]string, error) {
				return nil, errors.New("wdqs unavailable")
			},
		},
	}
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if w.subjects != 1 {
		t.Errorf(
			"upserts = %d, want 1 (Q1 refreshed despite discovery failure)",
			w.subjects)
	}
}

func TestParsePageviews(t *testing.T) {
	// The REST metrics response carries one item per period; the signal is
	// the sum.
	raw := []byte(`{"items":[
		{"project":"en.wikipedia","article":"Angela_Merkel",` +
		`"granularity":"monthly","timestamp":"2026030100","views":120000},
		{"project":"en.wikipedia","article":"Angela_Merkel",` +
		`"granularity":"monthly","timestamp":"2026040100","views":80000}
	]}`)
	total, err := parsePageviews(raw)
	if err != nil {
		t.Fatalf("parsePageviews: %v", err)
	}
	if total != 200000 {
		t.Errorf("total = %d, want 200000", total)
	}
}

func TestParsePageviews_Empty(t *testing.T) {
	total, err := parsePageviews([]byte(`{"items":[]}`))
	if err != nil || total != 0 {
		t.Errorf("empty items = (%d,%v), want (0,nil)", total, err)
	}
}

func TestParseEntity_CapturesSitelinkTitles(t *testing.T) {
	// The per-language titles drive the pageview pass without extra fetches; the
	// non-Wikipedia commons sitelink is excluded like it is from Langs.
	raw := []byte(`{"entities":{"Q567":{
		"labels":{"en":{"value":"Angela Merkel"}},
		"sitelinks":{
			"enwiki":{"title":"Angela Merkel"},
			"frwiki":{"title":"Angela Merkel"},
			"commonswiki":{"title":"Category:Angela Merkel"}
		},
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}]}
	}}}`)
	got, err := parseEntity(raw, "Q567")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.Sitelinks["en"] != "Angela Merkel" ||
		got.Sitelinks["fr"] != "Angela Merkel" {
		t.Errorf("Sitelinks = %v (want en+fr titles)", got.Sitelinks)
	}
	if _, ok := got.Sitelinks["commons"]; ok {
		t.Errorf("Sitelinks should exclude commons: %v", got.Sitelinks)
	}
}

func TestSeedOne_RecordsPageviews(t *testing.T) {
	w := &fakeWriter{}
	s := &Seeder{
		Store:  w,
		Logger: discardLogger(),
		// de has no sitelink → skipped
		PageviewLangs:  []string{"en", "fr", "de"},
		PageviewWindow: 90 * 24 * time.Hour,
		Fetcher: fakeFetcher{
			entity: func(string) (EntityFacts, error) {
				return EntityFacts{
					QID: "Q1", IsHuman: true, LabelEn: "A Leader",
					EnwikiTitle: "A_Leader",
					Langs:       []string{"en", "fr"},
					Sitelinks: map[string]string{
						"en": "A Leader", "fr": "Un Dirigeant",
					},
				}, nil
			},
			summary: func(string, string) (Summary, error) {
				return Summary{Name: "A Leader", WikipediaURL: "u"}, nil
			},
			pageviews: func(lang, _ string) (int64, error) {
				return map[string]int64{"en": 5000, "fr": 1200}[lang], nil
			},
		},
	}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.pageviews) != 2 {
		t.Fatalf(
			"recorded %d pageview rows, want 2 (en+fr; de has no sitelink)",
			len(w.pageviews))
	}
	got := map[string]int64{}
	for _, pv := range w.pageviews {
		got[pv.lang] = pv.views
	}
	if got["en"] != 5000 || got["fr"] != 1200 {
		t.Errorf("pageviews = %v, want en=5000 fr=1200", got)
	}
}
func TestSeed_PageviewsDisabledSkipsPass(t *testing.T) {
	// No PageviewLangs configured: no pageview fetches, no global-views refresh.
	w := &fakeWriter{}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(string) (EntityFacts, error) {
			return EntityFacts{
				QID: "Q1", IsHuman: true, EnwikiTitle: "X",
				Langs: []string{"en"}, Sitelinks: map[string]string{"en": "X"},
			}, nil
		},
		summary: func(string, string) (Summary, error) {
			return Summary{Name: "X", WikipediaURL: "u"}, nil
		},
	}}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.pageviews) != 0 {
		t.Errorf("recorded %d pageviews with the pass disabled, want 0",
			len(w.pageviews))
	}
}

func TestLoadSeedItems_DedupesAndIsNonEmpty(t *testing.T) {
	items, err := loadSeedItems()
	if err != nil {
		t.Fatalf("loadSeedItems: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("embedded snapshot is empty — un_leaders.json not generated?")
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.QID == "" {
			t.Error("empty QID in snapshot")
		}
		if seen[it.QID] {
			t.Errorf("duplicate QID %s not deduped", it.QID)
		}
		seen[it.QID] = true
	}
}
