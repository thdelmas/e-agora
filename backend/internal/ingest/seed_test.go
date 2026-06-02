package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- pure parser tests -------------------------------------------------------

func TestParseEntity_Human(t *testing.T) {
	raw := []byte(`{"entities":{"Q567":{
		"labels":{"en":{"language":"en","value":"Angela Merkel"}},
		"sitelinks":{
			"enwiki":{"site":"enwiki","title":"Angela Merkel"},
			"frwiki":{"site":"frwiki","title":"Angela Merkel"},
			"commonswiki":{"site":"commonswiki","title":"Category:Angela Merkel"}
		},
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}]}
	}}}`)
	got, err := parseEntity(raw, "Q567")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if !got.IsHuman {
		t.Error("expected IsHuman = true")
	}
	if got.LabelEn != "Angela Merkel" {
		t.Errorf("LabelEn = %q", got.LabelEn)
	}
	if got.EnwikiTitle != "Angela Merkel" {
		t.Errorf("EnwikiTitle = %q", got.EnwikiTitle)
	}
	if len(got.Langs) != 2 || got.Langs[0] != "en" || got.Langs[1] != "fr" {
		t.Errorf("Langs = %v (want [en fr], commonswiki excluded)", got.Langs)
	}
}

func TestParseEntity_NotHuman(t *testing.T) {
	raw := []byte(`{"entities":{"Q183":{
		"labels":{"en":{"value":"Germany"}},
		"sitelinks":{"enwiki":{"title":"Germany"}},
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q6256"}}}}]}
	}}}`)
	got, err := parseEntity(raw, "Q183")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.IsHuman {
		t.Error("a country must not be flagged as human")
	}
}

func TestParseEntity_Redirect(t *testing.T) {
	// Requested Q1 but the response carries the canonical Q2 (merged).
	raw := []byte(`{"entities":{"Q2":{"labels":{"en":{"value":"X"}},"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}]},"sitelinks":{"enwiki":{"title":"X"}}}}}`)
	got, err := parseEntity(raw, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.QID != "Q2" || !got.IsHuman {
		t.Errorf("redirect not resolved: %+v", got)
	}
}

func TestParseEntity_MixedClaimValueTypes(t *testing.T) {
	// A real entity has many claims whose datavalue.value is a string (P18
	// image), a number, or a typed object (P569 time) — not the item shape.
	// Parsing must tolerate all of them and still read P31.
	raw := []byte(`{"entities":{"Q1":{
		"labels":{"en":{"value":"Someone"}},
		"sitelinks":{"enwiki":{"title":"Someone"}},
		"claims":{
			"P18":[{"mainsnak":{"datavalue":{"value":"Portrait.jpg"}}}],
			"P569":[{"mainsnak":{"datavalue":{"value":{"time":"+1970-01-01T00:00:00Z"}}}}],
			"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}]
		}
	}}}`)
	got, err := parseEntity(raw, "Q1")
	if err != nil {
		t.Fatalf("parseEntity must tolerate mixed value types: %v", err)
	}
	if !got.IsHuman {
		t.Error("expected IsHuman = true despite string/object claim values")
	}
}

func TestParseEntity_DeathDate(t *testing.T) {
	// P570 (date of death) is read alongside P31; its presence marks the subject
	// deceased and pins the death date for display.
	raw := []byte(`{"entities":{"Q1":{
		"labels":{"en":{"value":"A Late Leader"}},
		"sitelinks":{"enwiki":{"title":"A Late Leader"}},
		"claims":{
			"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}],
			"P570":[{"mainsnak":{"datavalue":{"value":{"time":"+2013-12-05T00:00:00Z","precision":11}}}}]
		}
	}}}`)
	got, err := parseEntity(raw, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.DiedAt != "2013-12-05" {
		t.Errorf("DiedAt = %q, want 2013-12-05", got.DiedAt)
	}
}

func TestParseEntity_LivingHasNoDeathDate(t *testing.T) {
	raw := []byte(`{"entities":{"Q1":{
		"labels":{"en":{"value":"A Living Leader"}},
		"sitelinks":{"enwiki":{"title":"A Living Leader"}},
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}]}
	}}}`)
	got, err := parseEntity(raw, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.DiedAt != "" {
		t.Errorf("DiedAt = %q, want empty for a living subject", got.DiedAt)
	}
}

func TestWikidataDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"+2013-12-05T00:00:00Z", "2013-12-05", true}, // day precision
		{"+1881-00-00T00:00:00Z", "1881-01-01", true}, // year precision → clamp to Jan 1
		{"+1990-06-00T00:00:00Z", "1990-06-01", true}, // month precision → clamp day
		{"-0044-03-15T00:00:00Z", "", false},          // BCE — not handled
		{"2013-12-05T00:00:00Z", "", false},           // missing sign
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := wikidataDate(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("wikidataDate(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestWikiLang(t *testing.T) {
	cases := []struct {
		site string
		lang string
		ok   bool
	}{
		{"enwiki", "en", true},
		{"frwiki", "fr", true},
		{"be_x_oldwiki", "be-x-old", true},
		{"commonswiki", "", false},
		{"wikidatawiki", "", false},
		{"enwikisource", "", false},
		{"enwikiquote", "", false},
		{"wiki", "", false},
	}
	for _, c := range cases {
		lang, ok := wikiLang(c.site)
		if ok != c.ok || lang != c.lang {
			t.Errorf("wikiLang(%q) = (%q,%v), want (%q,%v)", c.site, lang, ok, c.lang, c.ok)
		}
	}
}

func TestParseSummary(t *testing.T) {
	raw := []byte(`{
		"type":"standard",
		"title":"Angela Merkel",
		"titles":{"normalized":"Angela Merkel"},
		"description":"Chancellor of Germany 2005–2021",
		"extract":"Angela Dorothea Merkel is a German politician.",
		"thumbnail":{"source":"https://upload.wikimedia.org/x.jpg"},
		"content_urls":{"desktop":{"page":"https://en.wikipedia.org/wiki/Angela_Merkel"}}
	}`)
	s, err := parseSummary(raw)
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if s.Name != "Angela Merkel" || s.Description != "Chancellor of Germany 2005–2021" {
		t.Errorf("name/desc = %q / %q", s.Name, s.Description)
	}
	if s.Extract != "Angela Dorothea Merkel is a German politician." {
		t.Errorf("extract = %q", s.Extract)
	}
	if s.ImageURL == "" || s.WikipediaURL == "" || s.Type != "standard" {
		t.Errorf("missing fields: %+v", s)
	}
}

func TestParseSummary_ExtractFallback(t *testing.T) {
	raw := []byte(`{"title":"X","extract":"First sentence here. Second one.","content_urls":{"desktop":{"page":"u"}}}`)
	s, err := parseSummary(raw)
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if s.Description != "First sentence here." {
		t.Errorf("extract fallback = %q", s.Description)
	}
}

func TestFirstSentence(t *testing.T) {
	if got := firstSentence("  Hello world. More."); got != "Hello world." {
		t.Errorf("firstSentence = %q", got)
	}
	if got := firstSentence(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

// --- seeder tests (fakes, no network) ----------------------------------------

type fakeWriter struct {
	count          int
	subjects       int
	qids           []string // returned by AllSubjectQIDs (the sync's existing pool)
	missingGeo     []string // returned by SubjectQIDsMissingGeo (the backfill work-list)
	translations   []translation
	pageviews      []pageview
	geos           []geo
	globalRefishes int
}

type translation struct{ lang, name, desc, extract, img, url string }
type pageview struct {
	subjectID int64
	lang      string
	views     int64
}
type geo struct {
	qid        string
	country    string
	continents []string
}

func (f *fakeWriter) CountSubjects(context.Context) (int, error)       { return f.count, nil }
func (f *fakeWriter) AllSubjectQIDs(context.Context) ([]string, error) { return f.qids, nil }
func (f *fakeWriter) UpsertSubject(_ context.Context, _, _, _ string, _ []string, _ string) (int64, error) {
	f.subjects++
	return int64(f.subjects), nil
}
func (f *fakeWriter) UpsertTranslation(_ context.Context, _ int64, lang, name, desc, extract, img, url string) error {
	f.translations = append(f.translations, translation{lang, name, desc, extract, img, url})
	return nil
}
func (f *fakeWriter) UpsertPageviews(_ context.Context, subjectID int64, lang string, views int64) error {
	f.pageviews = append(f.pageviews, pageview{subjectID, lang, views})
	return nil
}
func (f *fakeWriter) RefreshGlobalViews(context.Context) error { f.globalRefishes++; return nil }
func (f *fakeWriter) SetSubjectGeo(_ context.Context, qid, country string, continents []string) error {
	f.geos = append(f.geos, geo{qid, country, continents})
	return nil
}
func (f *fakeWriter) SubjectQIDsMissingGeo(context.Context) ([]string, error) {
	return f.missingGeo, nil
}

type fakeFetcher struct {
	entity    func(qid string) (EntityFacts, error)
	summary   func(lang, title string) (Summary, error)
	leaders   func() ([]string, error)
	pageviews func(lang, title string) (int64, error)
}

func (f fakeFetcher) Entity(_ context.Context, qid string) (EntityFacts, error) {
	return f.entity(qid)
}
func (f fakeFetcher) Summary(_ context.Context, lang, title string) (Summary, error) {
	return f.summary(lang, title)
}
func (f fakeFetcher) LeaderQIDs(context.Context) ([]string, error) {
	if f.leaders == nil {
		return nil, nil
	}
	return f.leaders()
}
func (f fakeFetcher) Pageviews(_ context.Context, lang, title string, _, _ time.Time) (int64, error) {
	if f.pageviews == nil {
		return 0, nil
	}
	return f.pageviews(lang, title)
}

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
	s := &Seeder{Store: &fakeWriter{}, Logger: discardLogger(), Fetcher: fakeFetcher{}}
	if err := s.Seed(context.Background(), "nonsense"); err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestSeedOne(t *testing.T) {
	humanFacts := EntityFacts{QID: "Q1", IsHuman: true, LabelEn: "A Leader", EnwikiTitle: "A_Leader", Langs: []string{"en", "fr"}}
	okSummary := Summary{Name: "A Leader", Description: "A politician.", ImageURL: "i", WikipediaURL: "u", Type: "standard"}

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
				entity: func(string) (EntityFacts, error) { return EntityFacts{}, ErrNotFound },
			},
			wantErr: errSkip, wantSubj: 0, wantTrans: 0,
		},
		{
			name: "summary failure still stores a degraded translation",
			fetcher: fakeFetcher{
				entity:  func(string) (EntityFacts, error) { return humanFacts, nil },
				summary: func(string, string) (Summary, error) { return Summary{}, errors.New("network") },
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
				t.Errorf("translations = %d, want %d", len(w.translations), c.wantTrans)
			}
		})
	}
}

func TestSeedOne_DegradedURLIsRealPage(t *testing.T) {
	w := &fakeWriter{}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(string) (EntityFacts, error) {
			return EntityFacts{QID: "Q1", IsHuman: true, EnwikiTitle: "Angela Merkel", Langs: []string{"en"}}, nil
		},
		summary: func(string, string) (Summary, error) { return Summary{}, errors.New("offline") },
	}}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.translations) != 1 || w.translations[0].url != "https://en.wikipedia.org/wiki/Angela_Merkel" {
		t.Errorf("degraded url = %+v", w.translations)
	}
}

func TestSyncOnce_RefreshesExistingAndDiscoversNew(t *testing.T) {
	// Two subjects already in the pool; discovery returns one duplicate (Q2) and
	// one genuinely new leader (Q3). Expect all three upserted: Q1+Q2 refreshed,
	// Q3 added.
	w := &fakeWriter{qids: []string{"Q1", "Q2"}}
	human := func(qid string) (EntityFacts, error) {
		return EntityFacts{QID: qid, IsHuman: true, LabelEn: qid, EnwikiTitle: qid, Langs: []string{"en"}}, nil
	}
	s := &Seeder{
		Store:  w,
		Logger: discardLogger(),
		Fetcher: fakeFetcher{
			entity:  human,
			summary: func(string, string) (Summary, error) { return Summary{Name: "n", WikipediaURL: "u"}, nil },
			leaders: func() ([]string, error) { return []string{"Q2", "Q3"}, nil },
		},
	}
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if w.subjects != 3 {
		t.Errorf("upserts = %d, want 3 (Q1,Q2 refreshed + Q3 discovered)", w.subjects)
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
				return EntityFacts{QID: qid, IsHuman: true, LabelEn: qid, EnwikiTitle: qid, Langs: []string{"en"}}, nil
			},
			summary: func(string, string) (Summary, error) { return Summary{Name: "n", WikipediaURL: "u"}, nil },
			leaders: func() ([]string, error) { return nil, errors.New("wdqs unavailable") },
		},
	}
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if w.subjects != 1 {
		t.Errorf("upserts = %d, want 1 (Q1 refreshed despite discovery failure)", w.subjects)
	}
}

func TestParsePageviews(t *testing.T) {
	// The REST metrics response carries one item per period; the signal is the sum.
	raw := []byte(`{"items":[
		{"project":"en.wikipedia","article":"Angela_Merkel","granularity":"monthly","timestamp":"2026030100","views":120000},
		{"project":"en.wikipedia","article":"Angela_Merkel","granularity":"monthly","timestamp":"2026040100","views":80000}
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
	if got.Sitelinks["en"] != "Angela Merkel" || got.Sitelinks["fr"] != "Angela Merkel" {
		t.Errorf("Sitelinks = %v (want en+fr titles)", got.Sitelinks)
	}
	if _, ok := got.Sitelinks["commons"]; ok {
		t.Errorf("Sitelinks should exclude commons: %v", got.Sitelinks)
	}
}

func TestSeedOne_RecordsPageviews(t *testing.T) {
	w := &fakeWriter{}
	s := &Seeder{
		Store:          w,
		Logger:         discardLogger(),
		PageviewLangs:  []string{"en", "fr", "de"}, // de has no sitelink → skipped
		PageviewWindow: 90 * 24 * time.Hour,
		Fetcher: fakeFetcher{
			entity: func(string) (EntityFacts, error) {
				return EntityFacts{
					QID: "Q1", IsHuman: true, LabelEn: "A Leader", EnwikiTitle: "A_Leader",
					Langs:     []string{"en", "fr"},
					Sitelinks: map[string]string{"en": "A Leader", "fr": "Un Dirigeant"},
				}, nil
			},
			summary:   func(string, string) (Summary, error) { return Summary{Name: "A Leader", WikipediaURL: "u"}, nil },
			pageviews: func(lang, _ string) (int64, error) { return map[string]int64{"en": 5000, "fr": 1200}[lang], nil },
		},
	}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.pageviews) != 2 {
		t.Fatalf("recorded %d pageview rows, want 2 (en+fr; de has no sitelink)", len(w.pageviews))
	}
	got := map[string]int64{}
	for _, pv := range w.pageviews {
		got[pv.lang] = pv.views
	}
	if got["en"] != 5000 || got["fr"] != 1200 {
		t.Errorf("pageviews = %v, want en=5000 fr=1200", got)
	}
}

func TestParseEntity_CountryAndContinent(t *testing.T) {
	// A person carries P27 (country); a country entity carries P30 (continent).
	person := []byte(`{"entities":{"Q1":{
		"labels":{"en":{"value":"A Leader"}},"sitelinks":{"enwiki":{"title":"A_Leader"}},
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}],
		          "P27":[{"mainsnak":{"datavalue":{"value":{"id":"Q142"}}}}]}}}}`)
	got, err := parseEntity(person, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.CountryQID != "Q142" {
		t.Errorf("CountryQID = %q, want Q142", got.CountryQID)
	}
	country := []byte(`{"entities":{"Q142":{"labels":{"en":{"value":"France"}},
		"sitelinks":{"enwiki":{"title":"France"}},
		"claims":{"P30":[{"mainsnak":{"datavalue":{"value":{"id":"Q46"}}}}]}}}}`)
	cgot, err := parseEntity(country, "Q142")
	if err != nil {
		t.Fatalf("parseEntity(country): %v", err)
	}
	if len(cgot.ContinentQIDs) != 1 || cgot.ContinentQIDs[0] != "Q46" || continentName["Q46"] != "Europe" {
		t.Errorf("continents = %v, want [Q46]→Europe", cgot.ContinentQIDs)
	}
}

// P27 selection picks the current citizenship: a deprecated claim or one with a
// P582 end-time qualifier (a former country) is skipped, and a preferred-rank
// claim wins over document order — so a post-1991 Russian leader resolves to
// Russia, not the Soviet Union it also lists.
func TestParseEntity_CurrentCitizenship(t *testing.T) {
	person := []byte(`{"entities":{"Q1":{
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}],
		          "P27":[
		            {"rank":"normal","mainsnak":{"datavalue":{"value":{"id":"Q15180"}}},
		             "qualifiers":{"P582":[{"datavalue":{"value":{"time":"+1991-12-26T00:00:00Z"}}}]}},
		            {"rank":"preferred","mainsnak":{"datavalue":{"value":{"id":"Q159"}}}}]}}}}`)
	got, err := parseEntity(person, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.CountryQID != "Q159" {
		t.Errorf("CountryQID = %q, want Q159 (Russia, not the ended Soviet Union)", got.CountryQID)
	}
}

// resolveCountry maps a country's P30 to region buckets: the first mappable one
// for most countries (so overseas-territory continents don't leak in), but EVERY
// mapped continent for the contiguous transcontinental states (Russia, Egypt, …).
// Australia is the Oceania regression: its P30 is Q55643/Q3960, never the Q538
// "Insular Oceania" the Pacific islands use.
func TestResolveCountry_Continents(t *testing.T) {
	cases := []struct {
		name      string
		countryID string   // P27 → which country entity we resolve
		p30       []string // that country's P30 claims
		wantLabel string
		wantConts []string
	}{
		{"france overseas territories → Europe only", "Q142", []string{"Q46", "Q15", "Q828", "Q538"}, "France", []string{"Europe"}},
		{"australia oceania qids", "Q408", []string{"Q55643", "Q3960"}, "Australia", []string{"Oceania"}},
		{"non-continent listed first", "Q30", []string{"Q828", "Q49"}, "USA", []string{"North America"}},
		{"no mappable continent", "Q123", []string{"Q828"}, "Nowhere", nil},
		{"russia is transcontinental", "Q159", []string{"Q46", "Q48"}, "Russia", []string{"Europe", "Asia"}},
		{"egypt is transcontinental", "Q79", []string{"Q15", "Q48"}, "Egypt", []string{"Africa", "Asia"}},
		{"transcontinental dedups repeats", "Q159", []string{"Q46", "Q46", "Q48"}, "Russia", []string{"Europe", "Asia"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Seeder{Logger: discardLogger(), Fetcher: fakeFetcher{
				entity: func(string) (EntityFacts, error) {
					return EntityFacts{QID: tc.countryID, LabelEn: tc.wantLabel, ContinentQIDs: tc.p30}, nil
				},
			}}
			got := s.resolveCountry(context.Background(), tc.countryID)
			if got.label != tc.wantLabel || !slices.Equal(got.continents, tc.wantConts) {
				t.Errorf("resolveCountry = %q/%v, want %q/%v", got.label, got.continents, tc.wantLabel, tc.wantConts)
			}
		})
	}
}

func TestSeedOne_ResolvesGeo(t *testing.T) {
	// The person's P27 → country fetch → continent, recorded once and cached.
	w := &fakeWriter{}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(qid string) (EntityFacts, error) {
			switch qid {
			case "Q142":
				return EntityFacts{QID: "Q142", LabelEn: "France", ContinentQIDs: []string{"Q46"}}, nil
			default:
				return EntityFacts{QID: qid, IsHuman: true, EnwikiTitle: "X", Langs: []string{"en"}, CountryQID: "Q142"}, nil
			}
		},
		summary: func(string, string) (Summary, error) { return Summary{Name: "X", WikipediaURL: "u"}, nil },
	}}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.geos) != 1 || w.geos[0].qid != "Q1" || w.geos[0].country != "France" ||
		!slices.Equal(w.geos[0].continents, []string{"Europe"}) {
		t.Errorf("geos = %+v, want one Q1/France/[Europe]", w.geos)
	}
}

// BackfillGeo resolves geo only for subjects missing a continent, keyed by QID —
// the self-heal path for figures that predate the pools feature (docs/10 §4).
func TestBackfillGeo_ResolvesMissingOnly(t *testing.T) {
	w := &fakeWriter{missingGeo: []string{"Q1", "Q2"}}
	s := &Seeder{Store: w, Logger: discardLogger(), Delay: 0, Fetcher: fakeFetcher{
		entity: func(qid string) (EntityFacts, error) {
			switch qid {
			case "Q1": // a leader → country Q142
				return EntityFacts{QID: "Q1", IsHuman: true, CountryQID: "Q142"}, nil
			case "Q2": // a stateless figure → no country, stays unscoped
				return EntityFacts{QID: "Q2", IsHuman: true}, nil
			case "Q142": // France → Europe
				return EntityFacts{QID: "Q142", LabelEn: "France", ContinentQIDs: []string{"Q46"}}, nil
			}
			return EntityFacts{QID: qid}, nil
		},
	}}
	if err := s.BackfillGeo(context.Background()); err != nil {
		t.Fatalf("BackfillGeo: %v", err)
	}
	if len(w.geos) != 1 || w.geos[0].qid != "Q1" || !slices.Equal(w.geos[0].continents, []string{"Europe"}) {
		t.Errorf("geos = %+v, want only Q1→[Europe] (Q2 unscoped)", w.geos)
	}
}

// A fully-resolved pool gives BackfillGeo an empty work-list and zero fetches.
func TestBackfillGeo_NoopWhenNothingMissing(t *testing.T) {
	w := &fakeWriter{missingGeo: nil}
	fetched := 0
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(qid string) (EntityFacts, error) { fetched++; return EntityFacts{QID: qid}, nil },
	}}
	if err := s.BackfillGeo(context.Background()); err != nil {
		t.Fatalf("BackfillGeo: %v", err)
	}
	if fetched != 0 || len(w.geos) != 0 {
		t.Errorf("fetched=%d geos=%d, want 0/0", fetched, len(w.geos))
	}
}

func TestSeed_PageviewsDisabledSkipsPass(t *testing.T) {
	// No PageviewLangs configured: no pageview fetches, no global-views refresh.
	w := &fakeWriter{}
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(string) (EntityFacts, error) {
			return EntityFacts{QID: "Q1", IsHuman: true, EnwikiTitle: "X", Langs: []string{"en"}, Sitelinks: map[string]string{"en": "X"}}, nil
		},
		summary: func(string, string) (Summary, error) { return Summary{Name: "X", WikipediaURL: "u"}, nil },
	}}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.pageviews) != 0 {
		t.Errorf("recorded %d pageviews with the pass disabled, want 0", len(w.pageviews))
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
