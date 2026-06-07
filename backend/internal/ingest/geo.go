package ingest

import "context"

// continentName maps a Wikidata continent QID (P30) to the stable English
// bucket the region pool groups countries into
// (docs/10-recognition-and-pools.md §4). Only the seven continents — a
// fixed, tiny set — so no per-country table is needed beyond resolving each
// country's P30 once. Oceania has several competing QIDs in Wikidata's P30
// statements (Q538 "Insular Oceania" on the Pacific islands, Q55643 "Oceania"
// and Q3960 "Australian continent" on Australia), so all three fold into the
// one bucket — otherwise Australia resolves to no region.
var continentName = map[string]string{
	"Q46":    "Europe",
	"Q48":    "Asia",
	"Q15":    "Africa",
	"Q49":    "North America",
	"Q18":    "South America",
	"Q538":   "Oceania",
	"Q55643": "Oceania",
	"Q3960":  "Oceania",
	"Q51":    "Antarctica",
}

// transcontinentalCountries are the contiguous transcontinental sovereign
// states whose figures belong in MORE THAN ONE continent pool (docs/10 §4) —
// a country split across a continental boundary on its core landmass. Everyone
// else keeps only the primary continent, because a country's P30 also lists
// its *overseas territories'* continents (France spans
// Europe+Africa+Oceania+Antarctica, Spain Europe+Africa, the USA North
// America+Oceania+Asia) and those must not scatter a leader across regions.
// This set is small and effectively static; the
// transcontinental few each list exactly their two core continents in P30, so
// keeping all *mapped* continents is correct only for members.
var transcontinentalCountries = map[string]bool{
	"Q159": true, // Russia      — Europe + Asia
	"Q43":  true, // Türkiye     — Europe + Asia
	"Q232": true, // Kazakhstan  — Europe + Asia
	"Q227": true, // Azerbaijan  — Europe + Asia
	"Q230": true, // Georgia     — Europe + Asia
	"Q79":  true, // Egypt       — Africa + Asia (Sinai)
	"Q252": true, // Indonesia   — Asia + Oceania (Western New Guinea)
}

// countryInfo is a resolved country's display label and the continent buckets
// its figures belong to (one for most countries, two for the transcontinental
// few).
type countryInfo struct {
	label      string
	continents []string
}

// resolveCountries resolves all of a figure's P27 country QIDs (a person can
// hold several citizenships) into the geo to store: the set of country labels
// they're a citizen of and the UNION of those countries' region-pool continents
// — so a multi-citizen lands in every one of their country pools and every
// continent those span (docs/10 §4). Order follows the input (preferred
// citizenships first); both sets are de-duplicated. Reuses the per-QID cache
// via resolveCountry, so a country shared across figures costs one fetch.
func (s *Seeder) resolveCountries(
	ctx context.Context, qids []string,
) (labels []string, continents []string) {
	seenLabel := map[string]bool{}
	seenCont := map[string]bool{}
	for _, qid := range qids {
		info := s.resolveCountry(ctx, qid)
		if info.label != "" && !seenLabel[info.label] {
			seenLabel[info.label] = true
			labels = append(labels, info.label)
		}
		for _, c := range info.continents {
			if !seenCont[c] {
				seenCont[c] = true
				continents = append(continents, c)
			}
		}
	}
	return labels, continents
}

// resolveCountry turns a person's P27 country QID into a display label and the
// region-pool continents it maps to, fetching the country entity once per
// distinct QID and caching the result on the Seeder for the pass — a pool of
// subjects from one country costs a single country fetch. Best-effort: an
// unresolved or continent-less country yields zero values (the subject just
// won't match a region pool).
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
		s.Logger.Warn("seed: country resolve failed", "country_qid", qid,
			"err", err)
	} else {
		info.label = facts.LabelEn
		info.continents = mappedContinents(
			facts.ContinentQIDs, transcontinentalCountries[qid])
	}
	s.countryCache[qid] = info
	return info
}

// mappedContinents reduces a country's raw P30 QIDs to region-pool buckets, in
// document order and de-duplicated. For a transcontinental country it keeps
// every distinct mapped continent; otherwise just the first (the primary
// continent), so overseas-territory continents don't leak a figure into the
// wrong region pool.
func mappedContinents(qids []string, all bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, qid := range qids {
		name := continentName[qid]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if !all {
			break
		}
	}
	return out
}
