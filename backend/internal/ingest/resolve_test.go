package ingest

import "testing"

func TestParseWikipediaURL(t *testing.T) {
	cases := []struct {
		raw         string
		lang, title string
		ok          bool
	}{
		{"https://en.wikipedia.org/wiki/Jacinda_Ardern", "en", "Jacinda_Ardern", true},
		{"https://fr.wikipedia.org/wiki/Emmanuel_Macron", "fr", "Emmanuel_Macron", true},
		{"https://en.m.wikipedia.org/wiki/Barack_Obama", "en", "Barack_Obama", true},
		{"https://de.wikipedia.org/wiki/Olaf_Scholz?foo=bar", "de", "Olaf_Scholz", true},
		{"https://en.wikipedia.org/wiki/Caf%C3%A9", "en", "Café", true},
		{"https://example.com/wiki/X", "", "", false},
		{"https://en.wikipedia.org/", "", "", false},
		{"not a url at all", "", "", false},
		{"https://www.wikipedia.org/wiki/X", "", "", false},
	}
	for _, c := range cases {
		lang, title, ok := ParseWikipediaURL(c.raw)
		if ok != c.ok || lang != c.lang || title != c.title {
			t.Errorf("ParseWikipediaURL(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.raw, lang, title, ok, c.lang, c.title, c.ok)
		}
	}
}
