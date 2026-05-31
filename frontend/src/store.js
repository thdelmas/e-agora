// A tiny reactive store for the visitor's anonymous session state (/api/me):
// contribution count, 24h access window, add allowance, and human-verified flag.
// Components read it directly so the nav, banner, and add button stay in sync
// without prop-drilling.
import { reactive, ref } from 'vue'
import { api } from './api/client'

const WELCOME_KEY = 'eagora_welcomed'

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
