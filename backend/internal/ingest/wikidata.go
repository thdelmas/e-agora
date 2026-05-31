package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// instanceOfHuman is the Wikidata QID for "human" (P31 → Q5), the R8 eligibility
// check for the pool.
const instanceOfHuman = "Q5"

// EntityFacts is the subset of a Wikidata entity ingestion needs.
type EntityFacts struct {
	QID         string
	IsHuman     bool     // P31 contains Q5
	LabelEn     string   // English label → canonical_name
	EnwikiTitle string   // sitelink title for en.wikipedia.org (empty if none)
	Langs       []string // Wikipedia language editions this subject has (sorted)
}

// Entity fetches and parses the Wikidata entity for a QID via the EntityData
// endpoint. A redirect (merged QID) resolves to whatever single entity the
// response carries.
func (c *Client) Entity(ctx context.Context, qid string) (EntityFacts, error) {
	u := c.WikidataBase + "/wiki/Special:EntityData/" + qid + ".json"
	var raw json.RawMessage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return EntityFacts{}, err
	}
	return parseEntity(raw, qid)
}

// SitelinkTitle returns the Wikipedia page title for a QID in a given language
// (used for lazy translation fills, docs/06 §Step 4). Empty if that language
// edition has no page.
func (c *Client) SitelinkTitle(ctx context.Context, qid, lang string) (string, error) {
	u := c.WikidataBase + "/wiki/Special:EntityData/" + qid + ".json"
	var raw json.RawMessage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return "", err
	}
	var doc entityDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("decode entity %s: %w", qid, err)
	}
	ent, ok := doc.Entities[qid]
	if !ok {
		for _, v := range doc.Entities {
			ent, ok = v, true
			break
		}
	}
	if !ok {
		return "", fmt.Errorf("entity %s absent from response", qid)
	}
	site := strings.ReplaceAll(lang, "-", "_") + "wiki"
	return ent.Sitelinks[site].Title, nil
}

// entityDoc mirrors the slice of EntityData we consume.
type entityDoc struct {
	Entities map[string]struct {
		Labels map[string]struct {
			Value string `json:"value"`
		} `json:"labels"`
		Sitelinks map[string]struct {
			Title string `json:"title"`
		} `json:"sitelinks"`
		Claims map[string][]struct {
			Mainsnak struct {
				DataValue struct {
					// value's JSON type varies by datatype (object for items,
					// string for images/external-ids, …) — defer parsing so a
					// non-item claim doesn't fail the whole document.
					Value json.RawMessage `json:"value"`
				} `json:"datavalue"`
			} `json:"mainsnak"`
		} `json:"claims"`
	} `json:"entities"`
}

// parseEntity extracts EntityFacts from a raw EntityData document. Pure (no I/O)
// so it is unit-testable against captured fixtures.
func parseEntity(raw []byte, qid string) (EntityFacts, error) {
	var doc entityDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return EntityFacts{}, fmt.Errorf("decode entity %s: %w", qid, err)
	}

	ent, ok := doc.Entities[qid]
	if !ok {
		// Followed a redirect: the canonical entity is under a different key.
		for k, v := range doc.Entities {
			qid, ent, ok = k, v, true
			break
		}
	}
	if !ok {
		return EntityFacts{}, fmt.Errorf("entity %s absent from response", qid)
	}

	facts := EntityFacts{QID: qid, LabelEn: ent.Labels["en"].Value}
	for _, claim := range ent.Claims["P31"] {
		var v struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(claim.Mainsnak.DataValue.Value, &v); err == nil && v.ID == instanceOfHuman {
			facts.IsHuman = true
			break
		}
	}
	facts.EnwikiTitle = ent.Sitelinks["enwiki"].Title

	langs := make([]string, 0, len(ent.Sitelinks))
	for site := range ent.Sitelinks {
		if lang, ok := wikiLang(site); ok {
			langs = append(langs, lang)
		}
	}
	sort.Strings(langs)
	facts.Langs = langs
	return facts, nil
}

// nonLangWikis are sitelink sites that end in "wiki" but are not per-language
// Wikipedia editions.
var nonLangWikis = map[string]bool{
	"commonswiki": true, "specieswiki": true, "metawiki": true,
	"wikidatawiki": true, "mediawikiwiki": true, "incubatorwiki": true,
	"foundationwiki": true, "wikimaniawiki": true, "outreachwiki": true,
	"testwiki": true, "test2wiki": true, "sourceswiki": true,
}

// wikiLang maps a Wikidata sitelink site (e.g. "enwiki") to a Wikipedia language
// code ("en"). It returns false for non-Wikipedia sites (…wikisource, …wikiquote)
// and the non-language wikis above. Wikidata's underscore variants are
// normalized to hyphen language tags (be_x_old → be-x-old).
func wikiLang(site string) (string, bool) {
	if !strings.HasSuffix(site, "wiki") || nonLangWikis[site] {
		return "", false
	}
	lang := strings.TrimSuffix(site, "wiki")
	if lang == "" {
		return "", false
	}
	return strings.ReplaceAll(lang, "_", "-"), true
}
