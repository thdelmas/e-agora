// A tiny reactive store for the visitor's anonymous session state (/api/me):
// contribution count, 24h access window, add allowance, and human-verified flag.
// Components read it directly so the nav, banner, and add button stay in sync
// without prop-drilling.
import { reactive, ref } from 'vue'
import { api } from './api/client'

const WELCOME_KEY = 'eagora_welcomed'
const DECEASED_KEY = 'eagora_include_deceased'
const REGION_KEY = 'eagora_pool_region'
const FAME_KEY = 'eagora_pool_fame'

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

// poolQuery builds the query flags for the current pool selection, omitting any
// axis left at its default so the URL stays clean (and the server treats an
// absent flag as "no filter").
export function poolQuery() {
  const q = {}
  if (includeDeceased.value) q.includeDeceased = 'true'
  if (poolRegion.value) q.region = poolRegion.value
  if (poolFameTop.value) q.fameTier = 'top'
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
