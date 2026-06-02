package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Pageviews returns the total view count for a Wikipedia article over [from, to]
// in one language edition, via the Wikimedia REST pageviews metrics API
// (docs/10-recognition-and-pools.md). Agent class is "user" so bot/crawler
// traffic doesn't inflate the recognition signal. A 404 (the article has no
// recorded views in the window, or the title doesn't resolve) is treated as zero,
// not an error — the caller still records a real "barely-read" signal.
func (c *Client) Pageviews(ctx context.Context, lang, title string, from, to time.Time) (int64, error) {
	project := lang + ".wikipedia.org"
	article := strings.ReplaceAll(title, " ", "_")
	u := c.MetricsBase + "/api/rest_v1/metrics/pageviews/per-article/" +
		project + "/all-access/user/" + url.PathEscape(article) +
		"/monthly/" + from.Format("20060102") + "/" + to.Format("20060102")

	var raw json.RawMessage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return parsePageviews(raw)
}

type pageviewsDoc struct {
	Items []struct {
		Views int64 `json:"views"`
	} `json:"items"`
}

// parsePageviews sums the per-period view counts in a REST metrics response.
// Pure (no I/O) so it is unit-testable against a captured fixture.
func parsePageviews(raw []byte) (int64, error) {
	var d pageviewsDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0, fmt.Errorf("decode pageviews: %w", err)
	}
	var total int64
	for _, it := range d.Items {
		total += it.Views
	}
	return total, nil
}
