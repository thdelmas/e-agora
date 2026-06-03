package ingest

import (
	"context"
	"slices"
	"testing"
)

func TestParseEntity_CountryAndContinent(t *testing.T) {
	// A person carries P27 (country); a country entity carries P30 (continent).
	person := []byte(`{"entities":{"Q1":{
		"labels":{"en":{"value":"A Leader"}},` +
		`"sitelinks":{"enwiki":{"title":"A_Leader"}},
		"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}],
		          "P27":[{"mainsnak":{"datavalue":{"value":{"id":"Q142"}}}}]}}}}`)
	got, err := parseEntity(person, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.CountryQID != "Q142" {
		t.Errorf("CountryQID = %q, want Q142", got.CountryQID)
	}
	country := []byte(`{"entities":{"Q142":{` +
		`"labels":{"en":{"value":"France"}},
		"sitelinks":{"enwiki":{"title":"France"}},
		"claims":{"P30":[{"mainsnak":{"datavalue":{"value":{"id":"Q46"}}}}]}}}}`)
	cgot, err := parseEntity(country, "Q142")
	if err != nil {
		t.Fatalf("parseEntity(country): %v", err)
	}
	if len(cgot.ContinentQIDs) != 1 || cgot.ContinentQIDs[0] != "Q46" ||
		continentName["Q46"] != "Europe" {
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
		            {"rank":"normal","mainsnak":` +
		`{"datavalue":{"value":{"id":"Q15180"}}},
		             "qualifiers":{"P582":[{"datavalue":` +
		`{"value":{"time":"+1991-12-26T00:00:00Z"}}}]}},
		            {"rank":"preferred","mainsnak":` +
		`{"datavalue":{"value":{"id":"Q159"}}}}]}}}}`)
	got, err := parseEntity(person, "Q1")
	if err != nil {
		t.Fatalf("parseEntity: %v", err)
	}
	if got.CountryQID != "Q159" {
		t.Errorf(
			"CountryQID = %q, want Q159 (Russia, not the ended Soviet Union)",
			got.CountryQID)
	}
}

// resolveCountry maps a country's P30 to region buckets: the first mappable
// one for most countries (so overseas-territory continents don't leak in), but
// EVERY mapped continent for the contiguous transcontinental states (Russia,
// Egypt, …). Australia is the Oceania regression: its P30 is Q55643/Q3960,
// never the Q538 "Insular Oceania" the Pacific islands use.
func TestResolveCountry_Continents(t *testing.T) {
	cases := []struct {
		name      string
		countryID string   // P27 → which country entity we resolve
		p30       []string // that country's P30 claims
		wantLabel string
		wantConts []string
	}{
		{
			"france overseas territories → Europe only", "Q142",
			[]string{"Q46", "Q15", "Q828", "Q538"}, "France",
			[]string{"Europe"},
		},
		{
			"australia oceania qids", "Q408",
			[]string{"Q55643", "Q3960"}, "Australia", []string{"Oceania"},
		},
		{
			"non-continent listed first", "Q30",
			[]string{"Q828", "Q49"}, "USA", []string{"North America"},
		},
		{"no mappable continent", "Q123", []string{"Q828"}, "Nowhere", nil},
		{
			"russia is transcontinental", "Q159",
			[]string{"Q46", "Q48"}, "Russia", []string{"Europe", "Asia"},
		},
		{
			"egypt is transcontinental", "Q79",
			[]string{"Q15", "Q48"}, "Egypt", []string{"Africa", "Asia"},
		},
		{
			"transcontinental dedups repeats", "Q159",
			[]string{"Q46", "Q46", "Q48"}, "Russia",
			[]string{"Europe", "Asia"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Seeder{Logger: discardLogger(), Fetcher: fakeFetcher{
				entity: func(string) (EntityFacts, error) {
					return EntityFacts{
						QID: tc.countryID, LabelEn: tc.wantLabel,
						ContinentQIDs: tc.p30,
					}, nil
				},
			}}
			got := s.resolveCountry(context.Background(), tc.countryID)
			if got.label != tc.wantLabel ||
				!slices.Equal(got.continents, tc.wantConts) {
				t.Errorf("resolveCountry = %q/%v, want %q/%v",
					got.label, got.continents, tc.wantLabel, tc.wantConts)
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
				return EntityFacts{
					QID: "Q142", LabelEn: "France",
					ContinentQIDs: []string{"Q46"},
				}, nil
			default:
				return EntityFacts{
					QID: qid, IsHuman: true, EnwikiTitle: "X",
					Langs: []string{"en"}, CountryQID: "Q142",
				}, nil
			}
		},
		summary: func(string, string) (Summary, error) {
			return Summary{Name: "X", WikipediaURL: "u"}, nil
		},
	}}
	if err := s.seedOne(context.Background(), seedItem{QID: "Q1"}); err != nil {
		t.Fatalf("seedOne: %v", err)
	}
	if len(w.geos) != 1 || w.geos[0].qid != "Q1" ||
		w.geos[0].country != "France" ||
		!slices.Equal(w.geos[0].continents, []string{"Europe"}) {
		t.Errorf("geos = %+v, want one Q1/France/[Europe]", w.geos)
	}
}

// BackfillGeo resolves geo only for subjects missing a continent, keyed by
// QID — the self-heal path for figures that predate the pools feature
// (docs/10 §4).
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
				return EntityFacts{
					QID: "Q142", LabelEn: "France",
					ContinentQIDs: []string{"Q46"},
				}, nil
			}
			return EntityFacts{QID: qid}, nil
		},
	}}
	if err := s.BackfillGeo(context.Background()); err != nil {
		t.Fatalf("BackfillGeo: %v", err)
	}
	if len(w.geos) != 1 || w.geos[0].qid != "Q1" ||
		!slices.Equal(w.geos[0].continents, []string{"Europe"}) {
		t.Errorf("geos = %+v, want only Q1→[Europe] (Q2 unscoped)", w.geos)
	}
}

// A fully-resolved pool gives BackfillGeo an empty work-list and zero fetches.
func TestBackfillGeo_NoopWhenNothingMissing(t *testing.T) {
	w := &fakeWriter{missingGeo: nil}
	fetched := 0
	s := &Seeder{Store: w, Logger: discardLogger(), Fetcher: fakeFetcher{
		entity: func(qid string) (EntityFacts, error) {
			fetched++
			return EntityFacts{QID: qid}, nil
		},
	}}
	if err := s.BackfillGeo(context.Background()); err != nil {
		t.Fatalf("BackfillGeo: %v", err)
	}
	if fetched != 0 || len(w.geos) != 0 {
		t.Errorf("fetched=%d geos=%d, want 0/0", fetched, len(w.geos))
	}
}
