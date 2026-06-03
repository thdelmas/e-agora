package ingest

import (
	"io"
	"log/slog"
	"testing"
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
	raw := []byte(`{"entities":{"Q2":{"labels":{"en":{"value":"X"}},` +
		`"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q5"}}}}]},` +
		`"sitelinks":{"enwiki":{"title":"X"}}}}}`)
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
			"P569":[{"mainsnak":{"datavalue":{"value":` +
		`{"time":"+1970-01-01T00:00:00Z"}}}}],
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
			"P570":[{"mainsnak":{"datavalue":{"value":` +
		`{"time":"+2013-12-05T00:00:00Z","precision":11}}}}]
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
		// year precision → clamp to Jan 1
		{"+1881-00-00T00:00:00Z", "1881-01-01", true},
		// month precision → clamp day
		{"+1990-06-00T00:00:00Z", "1990-06-01", true},
		{"-0044-03-15T00:00:00Z", "", false}, // BCE — not handled
		{"2013-12-05T00:00:00Z", "", false},  // missing sign
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := wikidataDate(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("wikidataDate(%q) = (%q,%v), want (%q,%v)",
				c.in, got, ok, c.want, c.ok)
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
			t.Errorf("wikiLang(%q) = (%q,%v), want (%q,%v)",
				c.site, lang, ok, c.lang, c.ok)
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
		"content_urls":{"desktop":` +
		`{"page":"https://en.wikipedia.org/wiki/Angela_Merkel"}}
	}`)
	s, err := parseSummary(raw)
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if s.Name != "Angela Merkel" ||
		s.Description != "Chancellor of Germany 2005–2021" {
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
	raw := []byte(`{"title":"X",` +
		`"extract":"First sentence here. Second one.",` +
		`"content_urls":{"desktop":{"page":"u"}}}`)
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
