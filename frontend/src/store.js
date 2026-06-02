// A tiny reactive store for the visitor's anonymous session state (/api/me):
// contribution count, 24h access window, add allowance, and human-verified flag.
// Components read it directly so the nav, banner, and add button stay in sync
// without prop-drilling.
import { reactive, ref } from 'vue'
import { api } from './api/client'

const WELCOME_KEY = 'eagora_welcomed'
const DECEASED_KEY = 'eagora_include_deceased'
const REGION_KEY = 'eagora_pool_region'
const COUNTRY_KEY = 'eagora_pool_country'
const FAME_KEY = 'eagora_pool_fame'
const HOME_KEY = 'eagora_home_region'
const HOME_COUNTRY_KEY = 'eagora_home_country'

// The continents the region pool offers; '' is the whole world (no region
// filter). Mirrors the backend's continentName buckets (docs/10 §4).
export const REGIONS = ['', 'Europe', 'Asia', 'Africa', 'North America', 'South America', 'Oceania']

// Whether the visitor has seen the one-time welcome + privacy note. Kept in
// localStorage only (never sent to the server), so a returning visitor isn't
// shown it again — consistent with the anonymity promise it makes.
export const welcomeSeen = ref(readWelcome())

function readWelcome() {
  try {
    return localStorage.getItem(WELCOME_KEY) === '1'
  } catch {
    return false // private mode / storage disabled: just show it again
  }
}

// acknowledgeWelcome dismisses the welcome step and remembers it locally.
export function acknowledgeWelcome() {
  welcomeSeen.value = true
  try {
    localStorage.setItem(WELCOME_KEY, '1')
  } catch {
    // No persistence available; they'll see it again next visit. Harmless.
  }
}

// Whether the viewer has opted to include people who have died in the rankings
// and matchups. Default off (the living only). Persisted locally so the choice
// holds across the leaderboard and the voting view and survives a reload — it's
// a display preference, never sent to or stored by the server beyond the
// per-request query flag.
export const includeDeceased = ref(readIncludeDeceased())

function readIncludeDeceased() {
  try {
    return localStorage.getItem(DECEASED_KEY) === '1'
  } catch {
    return false
  }
}

// setIncludeDeceased records the viewer's deceased-filter choice.
export function setIncludeDeceased(on) {
  includeDeceased.value = on
  try {
    localStorage.setItem(DECEASED_KEY, on ? '1' : '0')
  } catch {
    // No persistence; the preference lasts this session only. Harmless.
  }
}

// The visitor-selected pool (docs/10 §4): a region (continent) and a "famous
// only" tier. Like the deceased filter, these scope *which* figures are drawn and
// ranked — never a separate ranking — and are display preferences persisted
// locally, sent only as per-request query flags.
function readLS(key, fallback) {
  try {
    return localStorage.getItem(key) ?? fallback
  } catch {
    return fallback
  }
}

export const poolRegion = ref(REGIONS.includes(readLS(REGION_KEY, '')) ? readLS(REGION_KEY, '') : '')
export const poolFameTop = ref(readLS(FAME_KEY, '0') === '1')

export function setPoolRegion(region) {
  poolRegion.value = REGIONS.includes(region) ? region : ''
  try {
    localStorage.setItem(REGION_KEY, poolRegion.value)
  } catch {}
}

export function setPoolFameTop(on) {
  poolFameTop.value = !!on
  try {
    localStorage.setItem(FAME_KEY, on ? '1' : '0')
  } catch {}
}

// The finer geographic scope (docs/10 §4): a single country — the same axis as
// poolRegion at a sharper zoom, so the picker offers one *or* the other (choosing
// a country clears the region and vice versa). The value is the country's English
// label, exactly what the server stores and filters on. Persisted locally and
// sent only as a per-request query flag, like the rest of the pool.
export const poolCountry = ref(readLS(COUNTRY_KEY, ''))

export function setPoolCountry(country) {
  poolCountry.value = country || ''
  try {
    localStorage.setItem(COUNTRY_KEY, poolCountry.value)
  } catch {}
}

// suggestCountry proposes a soft home *country* from the region subtag of the
// browser's volunteered locale (fr-FR → France) — never IP (docs/10 §6). The
// value is the Wikidata English label the backend stores, so an exact match
// leans the draw toward that country; an unmapped locale (or a label not in the
// pool) just yields no lean and the continent suggestion still applies. The map
// is small and best-effort — the visitor can always pick a country explicitly.
const COUNTRY_BY_CC = {
  FR: 'France', BE: 'Belgium', CH: 'Switzerland', DE: 'Germany', AT: 'Austria',
  ES: 'Spain', IT: 'Italy', PT: 'Portugal', NL: 'Netherlands', IE: 'Ireland',
  PL: 'Poland', SE: 'Sweden', NO: 'Norway', DK: 'Denmark', FI: 'Finland', GR: 'Greece',
  GB: 'United Kingdom', US: 'United States of America', CA: 'Canada',
  AU: 'Australia', NZ: 'New Zealand',
  BR: 'Brazil', MX: 'Mexico', AR: 'Argentina', CL: 'Chile', CO: 'Colombia', PE: 'Peru',
  IN: 'India', JP: 'Japan',
}

export function suggestCountry() {
  let locales = []
  try {
    locales = navigator.languages?.length ? navigator.languages : [navigator.language]
  } catch {
    locales = []
  }
  for (const raw of locales) {
    if (!raw) continue
    const cc = raw.split('-')[1]?.toUpperCase()
    if (cc && COUNTRY_BY_CC[cc]) return COUNTRY_BY_CC[cc]
  }
  return ''
}

// The visitor's home country (docs/10 §4): a *soft* draw lean toward their own
// country, finer than homeRegion and likewise non-excluding (unlike the strict
// poolCountry). Seeded from the locale when never set, so the silent majority who
// never open the picker still get a local-leaning draw; '' means no lean. Never
// inferred from IP; persisted locally and sent only as a per-request query flag.
export const homeCountry = ref(readHomeCountry())

function readHomeCountry() {
  const stored = readLS(HOME_COUNTRY_KEY, null)
  return stored !== null ? stored : suggestCountry()
}

export function setHomeCountry(country) {
  homeCountry.value = country || ''
  try {
    localStorage.setItem(HOME_COUNTRY_KEY, homeCountry.value)
  } catch {}
}

// The visitor's home region (docs/10 §4): the continent they follow most closely,
// chosen once at onboarding. Unlike poolRegion (a *strict* filter), this is a
// *soft* bias — the server leans the matchup draw toward this region without
// excluding anyone, so the cross-region discovery slot still bridges the pools.
// '' is the whole world (no lean). Never inferred from IP (docs/10 §6); persisted
// locally and sent only as a per-request query flag.
export const homeRegion = ref(REGIONS.includes(readLS(HOME_KEY, '')) ? readLS(HOME_KEY, '') : '')

// Whether the home-region question has been answered yet. A *missing* key means
// "never asked" → show the onboarding step; an empty string is itself a valid
// answer ("whole world"), so emptiness of the value can't stand in for "unset".
export const homeRegionChosen = ref(homeKeyPresent())

function homeKeyPresent() {
  try {
    return localStorage.getItem(HOME_KEY) !== null
  } catch {
    return false // no storage: treat as unanswered so the step still appears
  }
}

export function setHomeRegion(region) {
  homeRegion.value = REGIONS.includes(region) ? region : ''
  homeRegionChosen.value = true
  try {
    localStorage.setItem(HOME_KEY, homeRegion.value)
  } catch {}
}

// suggestRegion proposes a home region from the browser's *volunteered* language
// (navigator.languages) — never from IP (docs/10 §6) — to pre-highlight a chip in
// the onboarding step. A region subtag is the strong signal (pt-BR → South
// America, en-AU → Oceania); a bare language is a weak fallback; anything global
// or ambiguous (en, es, ar) yields '' so we don't presume. The visitor always
// confirms, so this only has to be a sensible default, not exact.
const REGION_BY_LOCALE = {
  'pt-br': 'South America', 'pt-pt': 'Europe',
  'es-es': 'Europe', 'es-mx': 'North America',
  'es-ar': 'South America', 'es-cl': 'South America', 'es-co': 'South America',
  'es-pe': 'South America', 'es-ve': 'South America', 'es-bo': 'South America',
  'es-ec': 'South America', 'es-py': 'South America', 'es-uy': 'South America',
  'en-us': 'North America', 'en-ca': 'North America',
  'en-gb': 'Europe', 'en-ie': 'Europe',
  'en-au': 'Oceania', 'en-nz': 'Oceania',
  'en-in': 'Asia', 'en-za': 'Africa', 'en-ng': 'Africa', 'en-ke': 'Africa',
  'fr-ca': 'North America', 'fr-fr': 'Europe', 'fr-be': 'Europe', 'fr-ch': 'Europe',
  'ar-eg': 'Africa', 'ar-ma': 'Africa', 'ar-dz': 'Africa', 'ar-tn': 'Africa',
  'ar-sa': 'Asia', 'ar-ae': 'Asia', 'ar-iq': 'Asia',
}
const REGION_BY_LANG = {
  de: 'Europe', it: 'Europe', nl: 'Europe', pl: 'Europe', ru: 'Europe',
  uk: 'Europe', sv: 'Europe', tr: 'Europe', el: 'Europe', cs: 'Europe',
  ro: 'Europe', hu: 'Europe', fi: 'Europe', da: 'Europe', no: 'Europe', fr: 'Europe',
  zh: 'Asia', ja: 'Asia', ko: 'Asia', hi: 'Asia', fa: 'Asia', id: 'Asia',
  vi: 'Asia', th: 'Asia', bn: 'Asia', ta: 'Asia', ur: 'Asia',
}

export function suggestRegion() {
  let locales = []
  try {
    locales = navigator.languages?.length ? navigator.languages : [navigator.language]
  } catch {
    locales = []
  }
  for (const raw of locales) {
    if (!raw) continue
    const loc = raw.toLowerCase()
    if (REGION_BY_LOCALE[loc]) return REGION_BY_LOCALE[loc]
    const primary = loc.split('-')[0]
    if (REGION_BY_LANG[primary]) return REGION_BY_LANG[primary]
  }
  return ''
}

// poolQuery builds the query flags for the current pool selection, omitting any
// axis left at its default so the URL stays clean (and the server treats an
// absent flag as "no filter").
export function poolQuery() {
  const q = {}
  if (includeDeceased.value) q.includeDeceased = 'true'
  if (poolRegion.value) q.region = poolRegion.value
  if (poolCountry.value) q.country = poolCountry.value
  if (poolFameTop.value) q.fameTier = 'top'
  if (homeRegion.value) q.home = homeRegion.value
  if (homeCountry.value) q.homeCountry = homeCountry.value
  return q
}

export const me = reactive({
  loaded: false,
  contributions: 0,
  hasAccess: false,
  accessExpiresAt: null,
  canAdd: false,
  humanVerified: false,
  humanVerifiedUntil: null,
})

// refreshMe pulls authoritative state from the server.
export async function refreshMe() {
  try {
    const m = await api.me()
    Object.assign(me, m)
  } catch {
    // Leave prior state; the server stays authoritative on the next call.
  } finally {
    me.loaded = true
  }
}

// applyVote folds a vote response into the store without an extra round-trip:
// contributions tick up and the (fixed) 24h access window is reflected.
export function applyVote(res) {
  if (typeof res.contributions === 'number') me.contributions = res.contributions
  if (res.accessTokenExpiresAt) {
    me.hasAccess = true
    me.accessExpiresAt = res.accessTokenExpiresAt
  }
}
