package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// leaderSPARQL asks the Wikidata Query Service for the *current* head of state
// (P35) and head of government (P6) of every UN member state (P463 = Q1065)
// plus the two UN observer states — Holy See (Q237) and State of Palestine
// (Q219060), which aren't P463 members (docs/06-wikipedia-ingestion.md
// §Step 1). It returns person QIDs; statements with an end-time qualifier
// (P582 — former office) are excluded so only sitting leaders come back. This
// is the same query that
// generated the committed un_leaders.json snapshot; the daily sync re-runs it
// live to discover newly-elected leaders.
const leaderSPARQL = `SELECT DISTINCT ?person WHERE {
  { ?country wdt:P463 wd:Q1065 } UNION ` +
	`{ VALUES ?country { wd:Q237 wd:Q219060 } }
  { ?country p:P35 ?st } UNION { ?country p:P6 ?st }
  ?st ps:P35|ps:P6 ?person .
  FILTER NOT EXISTS { ?st pq:P582 ?end }
}`

// LeaderQIDs runs leaderSPARQL against WDQS and returns the deduped person QIDs
// of sitting UN heads of state/government. WDQS can be slow or rate-limited, so
// the daily sync treats an error here as non-fatal (it refreshes the existing
// pool regardless) — discovery is best-effort.
func (c *Client) LeaderQIDs(ctx context.Context) ([]string, error) {
	u := c.WDQSBase + "/sparql?format=json&query=" +
		url.QueryEscape(leaderSPARQL)
	var raw json.RawMessage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return nil, fmt.Errorf("leader sparql: %w", err)
	}
	return parseLeaderResponse(raw)
}

// parseLeaderResponse extracts deduped QIDs from a SPARQL JSON results
// document. Pure (no I/O) so it is unit-testable against a captured response.
func parseLeaderResponse(raw []byte) ([]string, error) {
	var doc struct {
		Results struct {
			Bindings []struct {
				Person struct {
					Value string `json:"value"`
				} `json:"person"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode sparql results: %w", err)
	}
	out := make([]string, 0, len(doc.Results.Bindings))
	seen := make(map[string]bool, len(doc.Results.Bindings))
	for _, b := range doc.Results.Bindings {
		qid := qidFromURI(b.Person.Value)
		if qid == "" || seen[qid] {
			continue
		}
		seen[qid] = true
		out = append(out, qid)
	}
	return out, nil
}

// qidFromURI pulls the trailing entity id from a Wikidata entity URI
// ("http://www.wikidata.org/entity/Q567" → "Q567"), returning "" for anything
// that isn't a Q-number.
func qidFromURI(uri string) string {
	// LastIndexByte returns -1 when there's no slash, so uri[i+1:] is the whole
	// string — a bare "Q42" parses as itself.
	qid := uri[strings.LastIndexByte(uri, '/')+1:]
	if len(qid) < 2 || qid[0] != 'Q' {
		return ""
	}
	for _, r := range qid[1:] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return qid
}
