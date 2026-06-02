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
	QID          string
	IsHuman      bool              // P31 contains Q5
	LabelEn      string            // English label → canonical_name
	EnwikiTitle  string            // sitelink title for en.wikipedia.org (empty if none)
	Langs        []string          // Wikipedia language editions this subject has (sorted)
	Sitelinks    map[string]string // lang → Wikipedia page title (drives the pageview pass without extra fetches, docs/10)
	DiedAt       string            // P570 date of death, normalized YYYY-MM-DD; "" if living/unknown
	CountryQID    string            // P27 country of citizenship (first claim) — the region pool axis (docs/10 §4)
	ContinentQIDs []string          // P30 continents (every item claim, in document order) — set when the entity is a country; the caller picks the first that maps to a known bucket
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
			// rank distinguishes Wikidata's preferred/normal/deprecated claims;
			// qualifiers carry per-claim facts like P582 (end time), which marks a
			// former country of citizenship (docs/06). Both drive P27 selection.
			Rank       string                       `json:"rank"`
			Qualifiers map[string][]json.RawMessage `json:"qualifiers"`
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
	// P570 (date of death): its mere presence marks the subject deceased; we keep
	// the first claim that carries a parseable Gregorian date. A "somevalue" snak
	// (known dead, date unknown) has no datavalue and is left unflagged — rare for
	// the documented public figures in this pool.
	for _, claim := range ent.Claims["P570"] {
		var v struct {
			Time string `json:"time"`
		}
		if err := json.Unmarshal(claim.Mainsnak.DataValue.Value, &v); err == nil {
			if d, ok := wikidataDate(v.Time); ok {
				facts.DiedAt = d
				break
			}
		}
	}
	// P27 (country of citizenship) drives the region pool; P30 (continent) is read
	// when this entity *is* a country, so resolving a person's country yields its
	// continent in the same fetch.
	//
	// P27 picks the *current* citizenship: a leader can carry a former one too — a
	// post-1991 Russian's P27 lists the Soviet Union (rank normal, P582 end-time)
	// alongside Russia (rank preferred, no end). We skip deprecated claims and any
	// with a P582 end-time qualifier, then prefer the "preferred"-rank claim,
	// falling back to the first normal one — otherwise document order alone would
	// resolve such figures to the defunct predecessor state.
	var firstNormalP27 string
	for _, claim := range ent.Claims["P27"] {
		if claim.Rank == "deprecated" || len(claim.Qualifiers["P582"]) > 0 {
			continue // deprecated or a former (ended) citizenship
		}
		var v struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(claim.Mainsnak.DataValue.Value, &v); err != nil || v.ID == "" {
			continue
		}
		if claim.Rank == "preferred" {
			facts.CountryQID = v.ID
			break
		}
		if firstNormalP27 == "" {
			firstNormalP27 = v.ID
		}
	}
	if facts.CountryQID == "" {
		facts.CountryQID = firstNormalP27
	}
	// continent keeps *all* non-deprecated claims (in document order) because a
	// country can list several — overseas-territory continents, or distinct
	// Wikidata QIDs for the same continent (e.g. Australia's P30 is Oceania +
	// "Australian continent", never the "Insular Oceania" QID the islands use).
	// resolveCountry picks the first that maps to a known bucket (or, for the
	// transcontinental states, every mapped one), so a stray non-primary claim
	// listed first doesn't mislabel the country.
	for _, claim := range ent.Claims["P30"] {
		if claim.Rank == "deprecated" {
			continue
		}
		var v struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(claim.Mainsnak.DataValue.Value, &v); err == nil && v.ID != "" {
			facts.ContinentQIDs = append(facts.ContinentQIDs, v.ID)
		}
	}

	facts.EnwikiTitle = ent.Sitelinks["enwiki"].Title

	langs := make([]string, 0, len(ent.Sitelinks))
	sitelinks := make(map[string]string, len(ent.Sitelinks))
	for site, sl := range ent.Sitelinks {
		if lang, ok := wikiLang(site); ok {
			langs = append(langs, lang)
			if sl.Title != "" {
				sitelinks[lang] = sl.Title
			}
		}
	}
	sort.Strings(langs)
	facts.Langs = langs
	facts.Sitelinks = sitelinks
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

// wikidataDate normalizes a Wikidata time value (e.g. "+1990-05-04T00:00:00Z")
// to a YYYY-MM-DD calendar date. Coarse-precision values zero out the month
// and/or day ("+1990-00-00T…" for a year-only date); we clamp those to January
// 1st so the result is a valid SQL DATE that still pins the death year. Negative
// (BCE) and non-4-digit-year values return false — irrelevant for this pool and
// not worth the calendar edge cases.
func wikidataDate(t string) (string, bool) {
	if !strings.HasPrefix(t, "+") {
		return "", false
	}
	t = t[1:]
	if len(t) < 10 || t[4] != '-' || t[7] != '-' {
		return "", false
	}
	year, month, day := t[0:4], t[5:7], t[8:10]
	if month == "00" {
		month = "01"
	}
	if day == "00" {
		day = "01"
	}
	return year + "-" + month + "-" + day, true
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
