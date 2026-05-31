package ingest

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// SearchResult is one autocomplete suggestion for adding a subject.
type SearchResult struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	ImageURL     string `json:"imageUrl"`
	WikipediaURL string `json:"wikipediaUrl"`
}

// Search proxies the Wikipedia REST search so a visitor can disambiguate before
// adding (docs/04-api.md GET /api/subjects/search). The chosen result's
// wikipediaUrl is then POSTed to /api/subjects.
func (c *Client) Search(ctx context.Context, lang, q string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	host := fmt.Sprintf(c.WikipediaTmpl, lang)
	u := host + "/w/rest.php/v1/search/page?" + url.Values{
		"q":     {q},
		"limit": {fmt.Sprintf("%d", limit)},
	}.Encode()

	var doc struct {
		Pages []struct {
			Key         string `json:"key"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Thumbnail   *struct {
				URL string `json:"url"`
			} `json:"thumbnail"`
		} `json:"pages"`
	}
	if err := c.getJSON(ctx, u, &doc); err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, len(doc.Pages))
	for _, p := range doc.Pages {
		img := ""
		if p.Thumbnail != nil {
			img = withScheme(p.Thumbnail.URL)
		}
		out = append(out, SearchResult{
			Title:        p.Title,
			Description:  p.Description,
			ImageURL:     img,
			WikipediaURL: host + "/wiki/" + url.PathEscape(p.Key),
		})
	}
	return out, nil
}

// withScheme upgrades protocol-relative thumbnail URLs (//upload.wikimedia.org/…).
func withScheme(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}
