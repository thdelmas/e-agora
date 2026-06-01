package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// userAgent identifies e-agora to Wikimedia per their API etiquette; a generic
// or empty agent may be blocked (docs/06-wikipedia-ingestion.md §Etiquette).
const userAgent = "e-agora/0.1 (https://github.com/thdelmas/e-agora; ingestion)"

// ErrNotFound is returned when an upstream resource resolves to 404 — the
// caller skips that subject (R2: no page ⇒ not eligible).
var ErrNotFound = errors.New("upstream: not found")

// Client talks to Wikidata (EntityData) and Wikipedia (REST summary). Base URLs
// are fields so tests can point them at a local server. It performs single
// requests; pacing/politeness is the caller's concern (the seeder sleeps).
type Client struct {
	HTTP          *http.Client
	WikidataBase  string // e.g. https://www.wikidata.org
	WikipediaTmpl string // host template with %s for the language, e.g. https://%s.wikipedia.org
	WDQSBase      string // Wikidata Query Service (SPARQL), e.g. https://query.wikidata.org
}

// NewClient returns a Client with sensible production defaults.
func NewClient() *Client {
	return &Client{
		HTTP:          &http.Client{Timeout: 20 * time.Second},
		WikidataBase:  "https://www.wikidata.org",
		WikipediaTmpl: "https://%s.wikipedia.org",
		WDQSBase:      "https://query.wikidata.org",
	}
}

// getJSON fetches url and decodes the JSON body into v. A 404 yields
// ErrNotFound; other non-2xx statuses are errors. Redirects (canonical title /
// QID) are followed by the default http.Client.
func (c *Client) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain a little of the body for context without unbounded reads.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, snippet)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// wikipediaSummaryURL builds the REST summary endpoint for a (lang, title).
func (c *Client) wikipediaSummaryURL(lang, title string) string {
	host := fmt.Sprintf(c.WikipediaTmpl, lang)
	return host + "/api/rest_v1/page/summary/" + url.PathEscape(title)
}
