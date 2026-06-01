package ingest

import "testing"

func TestParseLeaderResponse(t *testing.T) {
	// A WDQS JSON results doc: two leaders, one duplicate (a person who is both
	// HoS and HoG), and one malformed binding that must be dropped.
	raw := []byte(`{
		"head": {"vars": ["person"]},
		"results": {"bindings": [
			{"person": {"type": "uri", "value": "http://www.wikidata.org/entity/Q567"}},
			{"person": {"type": "uri", "value": "http://www.wikidata.org/entity/Q7747"}},
			{"person": {"type": "uri", "value": "http://www.wikidata.org/entity/Q567"}},
			{"person": {"type": "uri", "value": "http://www.wikidata.org/entity/NotAQid"}}
		]}
	}`)
	got, err := parseLeaderResponse(raw)
	if err != nil {
		t.Fatalf("parseLeaderResponse: %v", err)
	}
	if len(got) != 2 || got[0] != "Q567" || got[1] != "Q7747" {
		t.Errorf("got %v, want [Q567 Q7747] (deduped, malformed dropped)", got)
	}
}

func TestQidFromURI(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://www.wikidata.org/entity/Q567", "Q567"},
		{"https://www.wikidata.org/entity/Q219060", "Q219060"},
		{"Q42", "Q42"},
		{"http://www.wikidata.org/entity/P31", ""}, // property, not an item
		{"http://www.wikidata.org/entity/Q12x", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := qidFromURI(c.in); got != c.want {
			t.Errorf("qidFromURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
