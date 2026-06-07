// Thin fetch wrapper. `credentials: 'include'` ensures the anonymous session
// and 24h access-token cookies flow on every request
// (docs/04-api.md §Conventions).

async function request(path, { method = 'GET', body } = {}) {
  const res = await fetch(`/api${path}`, {
    method,
    credentials: 'include',
    // The matchup, leaderboard and stats all change as votes come in, so
    // never serve a stale heuristically-cached GET — always revalidate
    // against the API.
    cache: 'no-store',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const err = new Error(data.message || res.statusText)
    err.code = data.error
    err.status = res.status
    err.retryAfter = res.headers.get('Retry-After')
    throw err
  }
  return data
}

export const api = {
  me: () => request('/me'),
  // opts may carry { lang, includeDeceased } — both optional query flags.
  matchup: (opts = {}) => {
    const p = new URLSearchParams(opts).toString()
    return request(`/matchup${p ? `?${p}` : ''}`)
  },
  vote: (winnerId, loserId) =>
    request('/votes', { method: 'POST', body: { winnerId, loserId } }),
  leaderboard: (opts = {}) => {
    const p = new URLSearchParams(opts).toString()
    return request(`/leaderboard${p ? `?${p}` : ''}`)
  },
  humanChallenge: () => request('/human/challenge'),
  humanVerify: (challengeId, answer, timing) =>
    request('/human/verify', {
      method: 'POST',
      body: { challengeId, answer, timing },
    }),
  searchSubjects: (q, lang) =>
    request(
      `/subjects/search?q=${encodeURIComponent(q)}` +
        `${lang ? `&lang=${lang}` : ''}`,
    ),
  addSubject: (payload) =>
    request('/subjects', { method: 'POST', body: payload }),
  stats: (days) => request(`/stats${days ? `?days=${days}` : ''}`),
  // Countries with enough subjects to scope a pool, for the picker
  // (docs/10 §4).
  countries: () => request('/countries'),
  // Recall type-ahead over existing subjects, for the belonging step
  // (docs/11 §2).
  recall: (q) => request(`/subjects/recall?q=${encodeURIComponent(q)}`),
  // Record one recall for the active pool — the belonging signal. body is
  // { subjectId } for an existing figure or { url } for a Wikipedia page not
  // yet in the pool (ingested on first recall, docs/11 §3).
  propose: (body, opts = {}) => {
    const p = new URLSearchParams(opts).toString()
    return request(`/proposals${p ? `?${p}` : ''}`, { method: 'POST', body })
  },
  // Confirm/infirm a subject's membership in the active pool (docs/11 §7).
  // verdict is 'confirm' | 'infirm' | 'none' (retract); opts carries the pool
  // scope, same as propose(). Returns the refreshed { confirms, infirms,
  // score, viewerVerdict, excluded }.
  membership: (subjectId, verdict, opts = {}) => {
    const p = new URLSearchParams(opts).toString()
    return request(`/membership${p ? `?${p}` : ''}`, {
      method: 'POST',
      body: { subjectId, verdict },
    })
  },
  // Every geographic pool a subject is in, each with its why-reason and the
  // crowd's membership verdict — for the subject detail view (docs/11 §7).
  subjectPools: (id) => request(`/subjects/${id}/pools`),
}
