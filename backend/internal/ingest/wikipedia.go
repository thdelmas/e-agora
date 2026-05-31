package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Summary is the per-language display content pulled from the Wikipedia REST
// summary endpoint (docs/06-wikipedia-ingestion.md §Step 2).
type Summary struct {
	Name         string // titles.normalized (fallback: title)
	Description  string // description (fallback: first sentence of extract)
	ImageURL     string // thumbnail.source (may be empty)
	WikipediaURL string // content_urls.desktop.page (required, R2)
	Type         string // "standard" | "disambiguation" | …
}

// Summary fetches and parses the summary for a (lang, title).
func (c *Client) Summary(ctx context.Context, lang, title string) (Summary, error) {
	var raw json.RawMessage
	if err := c.getJSON(ctx, c.wikipediaSummaryURL(lang, title), &raw); err != nil {
		return Summary{}, err
	}
	s, err := parseSummary(raw)
	if err != nil {
		return Summary{}, fmt.Errorf("%s summary for %q: %w", lang, title, err)
	}
	return s, nil
}

type summaryDoc struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Titles struct {
		Normalized string `json:"normalized"`
	} `json:"titles"`
	Description string `json:"description"`
	Extract     string `json:"extract"`
	Thumbnail   struct {
		Source string `json:"source"`
	} `json:"thumbnail"`
	ContentURLs struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
}

// parseSummary extracts a Summary from a raw REST summary document. Pure (no I/O).
func parseSummary(raw []byte) (Summary, error) {
	var d summaryDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		return Summary{}, fmt.Errorf("decode summary: %w", err)
	}
	name := d.Titles.Normalized
	if name == "" {
		name = d.Title
	}
	desc := strings.TrimSpace(d.Description)
	if desc == "" {
		desc = firstSentence(d.Extract)
	}
	return Summary{
		Name:         name,
		Description:  desc,
		ImageURL:     d.Thumbnail.Source,
		WikipediaURL: d.ContentURLs.Desktop.Page,
		Type:         d.Type,
	}, nil
}

// firstSentence returns a short, one-line description derived from a Wikipedia
// extract: the text up to the first period, or a truncated prefix.
func firstSentence(extract string) string {
	s := strings.TrimSpace(extract)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '.'); i > 0 && i < 240 {
		return s[:i+1]
	}
	if len(s) > 240 {
		return strings.TrimSpace(s[:240]) + "…"
	}
	return s
}
