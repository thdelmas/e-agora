package ingest

import "context"

// continentName maps a Wikidata continent QID (P30) to the stable English bucket
// the region pool groups countries into (docs/10-recognition-and-pools.md §4).
// Only the seven continents — a fixed, tiny set — so no per-country table is
// needed beyond resolving each country's P30 once.
var continentName = map[string]string{
	"Q46":  "Europe",
	"Q48":  "Asia",
	"Q15":  "Africa",
	"Q49":  "North America",
	"Q18":  "South America",
	"Q538": "Oceania",
	"Q51":  "Antarctica",
}

// countryInfo is a resolved country's display label and continent.
type countryInfo struct {
	label     string
	continent string
}

// resolveCountry turns a person's P27 country QID into a display label and a
// continent name, fetching the country entity once per distinct QID and caching
// the result on the Seeder for the pass — a pool of subjects from one country
// costs a single country fetch. Best-effort: an unresolved or continent-less
// country yields zero values (the subject just won't match a region pool).
func (s *Seeder) resolveCountry(ctx context.Context, qid string) countryInfo {
	if qid == "" {
		return countryInfo{}
	}
	if s.countryCache == nil {
		s.countryCache = make(map[string]countryInfo)
	}
	if info, ok := s.countryCache[qid]; ok {
		return info
	}
	var info countryInfo
	if facts, err := s.Fetcher.Entity(ctx, qid); err != nil {
		s.Logger.Warn("seed: country resolve failed", "country_qid", qid, "err", err)
	} else {
		info.label = facts.LabelEn
		info.continent = continentName[facts.ContinentQID]
	}
	s.countryCache[qid] = info
	return info
}
