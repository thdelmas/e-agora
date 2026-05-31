// Package lang resolves the visitor's display language and applies the R9 rule:
// a matchup renders in the visitor's language only if BOTH subjects have it,
// otherwise English (the universal fallback). Never mixed (docs/04-api.md,
// docs/06-wikipedia-ingestion.md §Step 4).
package lang

import "strings"

// FromAccept extracts a Wikipedia language code from an Accept-Language header,
// using the primary subtag of the highest-priority entry (pt-BR → pt). It
// returns "" when the header is empty/unparseable. (v1 ignores q-weights beyond
// taking the first entry.)
func FromAccept(header string) string {
	if header == "" {
		return ""
	}
	first := header
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 { // strip ;q=…
		first = first[:i]
	}
	return normalize(first)
}

// Pick resolves the visitor's preferred code: an explicit override (?lang=) wins,
// then the Accept-Language header, then the configured fallback.
func Pick(override, acceptHeader, fallback string) string {
	if c := normalize(override); c != "" {
		return c
	}
	if c := FromAccept(acceptHeader); c != "" {
		return c
	}
	return normalize(fallback)
}

// Resolve applies R9 for a matchup pair: the display language is `visitor` iff
// both subjects have it, else `fallback`. fellBack is true when the visitor's
// language was dropped (drives the "shown in English" note).
func Resolve(visitor, fallback string, aLangs, bLangs []string) (display string, fellBack bool) {
	if visitor != "" && has(aLangs, visitor) && has(bLangs, visitor) {
		return visitor, false
	}
	return fallback, visitor != "" && visitor != fallback
}

func normalize(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

func has(langs []string, want string) bool {
	for _, l := range langs {
		if l == want {
			return true
		}
	}
	return false
}
