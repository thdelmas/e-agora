package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// SearchResult is one autocomplete suggestion for adding a subject.
type SearchResult struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	ImageURL     string `json:"imageUrl"`
	WikipediaURL string `json:"wikipediaUrl"`
}

// qidRe matches a bare Wikidata entity id (Q42).
var qidRe = regexp.MustCompile(`^Q[1-9][0-9]*$`)

// searchCand is a ranked Wikipedia hit paired with its Wikidata id, kept until
// the human filter decides whether to surface it.
type searchCand struct {
	res SearchResult
	qid string
}

// Search proposes addable subjects for a name query, restricted to people so a
// visitor only ever sees eligible candidates (docs/04-api.md GET
// /api/subjects/search; R8 — instance of human, P31=Q5).
//
// People used to be filtered inline on Wikipedia with the CirrusSearch keyword
// `haswbstatement:P31=Q5`, but that Wikidata-backed keyword stopped returning
// any results on the Wikipedias. So we now run two independent searches for the
// query in parallel and intersect them by Wikidata id:
//
//   - Wikipedia (the requested language) supplies ranked display data —
//     title, description, thumbnail, canonical URL — plus each hit's linked
//     Wikidata id.
//   - Wikidata (where `haswbstatement` still works) supplies the set of
//     matching ids that are human.
//
// A candidate is surfaced only if its id is in the human set. If the Wikidata
// lookup fails, we degrade to the unfiltered candidates rather than breaking
// search — the add endpoint still re-validates R8 on click.
func (c *Client) Search(
	ctx context.Context, lang, q string, limit int,
) ([]SearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	// Over-fetch: only a fraction of a plain name search is people (e.g. "trump"
	// also returns Trumpism, the Trump family, a $TRUMP coin…), so gather a
	// wider candidate pool to still fill `limit` suggestions after the human
	// filter.
	candidates := limit * 3
	if candidates > 50 {
		candidates = 50 // CirrusSearch limit ceiling for anonymous callers.
	}

	var (
		wg              sync.WaitGroup
		cands           []searchCand
		humans          map[string]bool
		candErr, humErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		cands, candErr = c.searchCandidates(ctx, lang, q, candidates)
	}()
	go func() {
		defer wg.Done()
		humans, humErr = c.humanQIDs(ctx, q, candidates)
	}()
	wg.Wait()

	if candErr != nil {
		return nil, candErr
	}

	results := make([]SearchResult, 0, limit)
	for _, cd := range cands {
		// humErr ⇒ Wikidata filter unavailable: fall back to unfiltered candidates.
		if humErr == nil && !humans[cd.qid] {
			continue
		}
		results = append(results, cd.res)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// searchCandidates runs the Wikipedia name search and returns ranked candidates
// with their linked Wikidata ids. generator=search keeps CirrusSearch ranking
// while prop=description|pageimages|info|pageprops enriches each hit with the
// short description, thumbnail, canonical URL and Wikidata id in one
// round-trip.
func (c *Client) searchCandidates(
	ctx context.Context, lang, q string, limit int,
) ([]searchCand, error) {
	host := fmt.Sprintf(c.WikipediaTmpl, lang)
	u := host + "/w/api.php?" + url.Values{
		"action":       {"query"},
		"format":       {"json"},
		"generator":    {"search"},
		"gsrsearch":    {q},
		"gsrnamespace": {"0"},
		"gsrlimit":     {fmt.Sprintf("%d", limit)},
		"prop":         {"description|pageimages|info|pageprops"},
		"ppprop":       {"wikibase_item|disambiguation"},
		"piprop":       {"thumbnail"},
		"pilimit":      {fmt.Sprintf("%d", limit)},
		"pithumbsize":  {"160"},
		"inprop":       {"url"},
		"redirects":    {"1"},
	}.Encode()

	var raw json.RawMessage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return nil, err
	}
	return parseSearchPages(raw)
}

// parseSearchPages turns a generator=search response into ranked candidates,
// dropping pages that can't be addable people anyway — disambiguation pages
// and pages with no linked Wikidata item (nothing to validate, and
// unaddable). Pure (no I/O) so it is unit-testable against captured fixtures.
func parseSearchPages(raw []byte) ([]searchCand, error) {
	var doc struct {
		Query struct {
			Pages map[string]struct {
				Index       int    `json:"index"` // search rank; the map is unordered
				Title       string `json:"title"`
				Description string `json:"description"`
				FullURL     string `json:"fullurl"`
				Thumbnail   *struct {
					Source string `json:"source"`
				} `json:"thumbnail"`
				PageProps struct {
					WikibaseItem   string  `json:"wikibase_item"`
					Disambiguation *string `json:"disambiguation"`
				} `json:"pageprops"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	// query.pages is keyed by page id, so restore the relevance order via index.
	indices := make([]int, 0, len(doc.Query.Pages))
	byIndex := make(map[int]searchCand, len(doc.Query.Pages))
	for _, p := range doc.Query.Pages {
		if p.PageProps.Disambiguation != nil ||
			!qidRe.MatchString(p.PageProps.WikibaseItem) {
			continue
		}
		img := ""
		if p.Thumbnail != nil {
			img = withScheme(p.Thumbnail.Source)
		}
		byIndex[p.Index] = searchCand{
			res: SearchResult{
				Title:        p.Title,
				Description:  p.Description,
				ImageURL:     img,
				WikipediaURL: p.FullURL,
			},
			qid: p.PageProps.WikibaseItem,
		}
		indices = append(indices, p.Index)
	}
	sort.Ints(indices)

	cands := make([]searchCand, 0, len(indices))
	for _, i := range indices {
		cands = append(cands, byIndex[i])
	}
	return cands, nil
}

// humanQIDs returns the set of Wikidata ids matching q that are instances of
// human (P31 → Q5). `haswbstatement` still works on its native repo
// (wikidata.org) even though it no longer does on the Wikipedias, and the
// entity page titles are themselves the ids — so one Action API search yields
// the human set.
func (c *Client) humanQIDs(
	ctx context.Context, q string, limit int,
) (map[string]bool, error) {
	u := c.WikidataBase + "/w/api.php?" + url.Values{
		"action":   {"query"},
		"format":   {"json"},
		"list":     {"search"},
		"srsearch": {q + " haswbstatement:P31=Q5"},
		"srlimit":  {fmt.Sprintf("%d", limit)},
		"srprop":   {""}, // titles (= QIDs) only; we don't need snippets
	}.Encode()

	var raw json.RawMessage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return nil, err
	}
	return parseHumanQIDs(raw)
}

// parseHumanQIDs reads the entity ids from a Wikidata list=search response
// (each hit's title is the QID). Pure (no I/O).
func parseHumanQIDs(raw []byte) (map[string]bool, error) {
	var doc struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode wikidata search: %w", err)
	}
	out := make(map[string]bool, len(doc.Query.Search))
	for _, s := range doc.Query.Search {
		if qidRe.MatchString(s.Title) {
			out[s.Title] = true
		}
	}
	return out, nil
}

// withScheme upgrades protocol-relative thumbnail URLs
// (//upload.wikimedia.org/…).
func withScheme(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}
