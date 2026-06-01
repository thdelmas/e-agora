package ingest

import "testing"

// parseSearchPages should restore relevance order, attach thumbnails/QIDs, and
// drop pages that can't be addable people (disambiguation, or no Wikidata item).
func TestParseSearchPages(t *testing.T) {
	raw := []byte(`{"query":{"pages":{
		"111":{"index":2,"title":"Pierre Curie","description":"physicist","fullurl":"https://en.wikipedia.org/wiki/Pierre_Curie",
			"thumbnail":{"source":"//upload.wikimedia.org/p.jpg"},"pageprops":{"wikibase_item":"Q37463"}},
		"222":{"index":1,"title":"Marie Curie","description":"physicist and chemist","fullurl":"https://en.wikipedia.org/wiki/Marie_Curie",
			"pageprops":{"wikibase_item":"Q7186"}},
		"333":{"index":3,"title":"Curie (disambiguation)","fullurl":"https://en.wikipedia.org/wiki/Curie",
			"pageprops":{"wikibase_item":"Q223850","disambiguation":""}},
		"444":{"index":4,"title":"Some stub","fullurl":"https://en.wikipedia.org/wiki/Stub","pageprops":{}}
	}}}`)

	cands, err := parseSearchPages(raw)
	if err != nil {
		t.Fatalf("parseSearchPages: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want 2 (disambiguation + item-less pages dropped)", len(cands))
	}
	// Relevance order: index 1 (Marie) before index 2 (Pierre).
	if cands[0].qid != "Q7186" || cands[0].res.Title != "Marie Curie" {
		t.Errorf("first candidate = %+v, want Marie Curie / Q7186", cands[0])
	}
	if cands[1].qid != "Q37463" {
		t.Errorf("second candidate qid = %q, want Q37463", cands[1].qid)
	}
	// Protocol-relative thumbnails are upgraded to https.
	if cands[1].res.ImageURL != "https://upload.wikimedia.org/p.jpg" {
		t.Errorf("ImageURL = %q, want https-upgraded", cands[1].res.ImageURL)
	}
}

func TestParseHumanQIDs(t *testing.T) {
	// Wikidata list=search: each hit's title is the entity id.
	raw := []byte(`{"query":{"search":[
		{"ns":0,"title":"Q7186","pageid":7150},
		{"ns":0,"title":"Q37463","pageid":37200}
	]}}`)
	humans, err := parseHumanQIDs(raw)
	if err != nil {
		t.Fatalf("parseHumanQIDs: %v", err)
	}
	if len(humans) != 2 || !humans["Q7186"] || !humans["Q37463"] {
		t.Errorf("humans = %v, want {Q7186, Q37463}", humans)
	}
	if humans["Q223850"] {
		t.Error("unexpected non-human QID present")
	}
}
