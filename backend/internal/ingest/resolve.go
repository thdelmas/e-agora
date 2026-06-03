package ingest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrBadInput means the add input wasn't a usable Wikipedia URL.
var ErrBadInput = errors.New("ingest: unrecognized Wikipedia URL")

// ParseWikipediaURL extracts (lang, title) from a Wikipedia article URL such as
// https://en.wikipedia.org/wiki/Jacinda_Ardern (also …m.wikipedia.org). Pure.
func ParseWikipediaURL(raw string) (lang, title string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !strings.HasSuffix(u.Host, "wikipedia.org") {
		return "", "", false
	}
	label := u.Host[:strings.IndexByte(u.Host, '.')]
	if label == "" || label == "www" {
		return "", "", false
	}
	const prefix = "/wiki/"
	if !strings.HasPrefix(u.EscapedPath(), prefix) {
		return "", "", false
	}
	t, err := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), prefix))
	if err != nil || t == "" {
		return "", "", false
	}
	return label, t, true
}

// ResolveWikipediaURL resolves an article URL to its Wikidata QID.
func (c *Client) ResolveWikipediaURL(
	ctx context.Context, raw string,
) (string, error) {
	lang, title, ok := ParseWikipediaURL(raw)
	if !ok {
		return "", ErrBadInput
	}
	return c.ResolveTitleToQID(ctx, lang, title)
}

// ResolveTitleToQID maps a (lang, title) to a QID via the pageprops API.
// Returns ErrNotFound when the title has no page / no linked Wikidata item.
func (c *Client) ResolveTitleToQID(
	ctx context.Context, lang, title string,
) (string, error) {
	q := url.Values{
		"action":    {"query"},
		"prop":      {"pageprops"},
		"ppprop":    {"wikibase_item"},
		"redirects": {"1"},
		"format":    {"json"},
		"titles":    {title},
	}
	u := fmt.Sprintf(c.WikipediaTmpl, lang) + "/w/api.php?" + q.Encode()

	var doc struct {
		Query struct {
			Pages map[string]struct {
				PageProps struct {
					WikibaseItem string `json:"wikibase_item"`
				} `json:"pageprops"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, u, &doc); err != nil {
		return "", err
	}
	for _, p := range doc.Query.Pages {
		if p.PageProps.WikibaseItem != "" {
			return p.PageProps.WikibaseItem, nil
		}
	}
	return "", ErrNotFound
}
