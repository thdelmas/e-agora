package ingest

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// SearchResult is one autocomplete suggestion for adding a subject.
type SearchResult struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	ImageURL     string `json:"imageUrl"`
	WikipediaURL string `json:"wikipediaUrl"`
}

// Search proxies a Wikipedia search restricted to people, so a visitor can
// disambiguate before adding (docs/04-api.md GET /api/subjects/search). The
// CirrusSearch `haswbstatement:P31=Q5` keyword limits matches to pages whose
// linked Wikidata item is an instance of human (Q5) — the same R8 eligibility
// rule the add endpoint enforces — so only addable subjects are ever proposed.
// The chosen result's wikipediaUrl is then POSTed to /api/subjects, which
// re-validates server-side.
func (c *Client) Search(ctx context.Context, lang, q string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	host := fmt.Sprintf(c.WikipediaTmpl, lang)
	// generator=search keeps CirrusSearch ranking while prop=description|
	// pageimages|info enriches each hit with the short description, thumbnail and
	// canonical URL in a single round-trip. inprop=url yields fullurl so we don't
	// have to reconstruct (and re-escape) the article path ourselves.
	u := host + "/w/api.php?" + url.Values{
		"action":       {"query"},
		"format":       {"json"},
		"generator":    {"search"},
		"gsrsearch":    {q + " haswbstatement:P31=Q5"},
		"gsrnamespace": {"0"},
		"gsrlimit":     {fmt.Sprintf("%d", limit)},
		"prop":         {"description|pageimages|info"},
		"piprop":       {"thumbnail"},
		"pithumbsize":  {"160"},
		"inprop":       {"url"},
		"redirects":    {"1"},
	}.Encode()

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
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, u, &doc); err != nil {
		return nil, err
	}

	// query.pages is keyed by page id, so restore the relevance order via index.
	pages := make([]SearchResult, 0, len(doc.Query.Pages))
	indices := make([]int, 0, len(doc.Query.Pages))
	byIndex := make(map[int]SearchResult, len(doc.Query.Pages))
	for _, p := range doc.Query.Pages {
		img := ""
		if p.Thumbnail != nil {
			img = withScheme(p.Thumbnail.Source)
		}
		byIndex[p.Index] = SearchResult{
			Title:        p.Title,
			Description:  p.Description,
			ImageURL:     img,
			WikipediaURL: p.FullURL,
		}
		indices = append(indices, p.Index)
	}
	sort.Ints(indices)
	for _, i := range indices {
		pages = append(pages, byIndex[i])
	}
	return pages, nil
}

// withScheme upgrades protocol-relative thumbnail URLs (//upload.wikimedia.org/…).
func withScheme(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}
