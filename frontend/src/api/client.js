// Thin fetch wrapper. `credentials: 'include'` ensures the anonymous session and
// 24h access-token cookies flow on every request (docs/04-api.md §Conventions).

async function request(path, { method = 'GET', body } = {}) {
  const res = await fetch(`/api${path}`, {
    method,
    credentials: 'include',
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
  matchup: (lang) => request(`/matchup${lang ? `?lang=${encodeURIComponent(lang)}` : ''}`),
  vote: (winnerId, loserId) => request('/votes', { method: 'POST', body: { winnerId, loserId } }),
  leaderboard: (opts = {}) => {
    const p = new URLSearchParams(opts).toString()
    return request(`/leaderboard${p ? `?${p}` : ''}`)
  },
  humanChallenge: () => request('/human/challenge'),
  humanVerify: (challengeId, answer) => request('/human/verify', { method: 'POST', body: { challengeId, answer } }),
  searchSubjects: (q, lang) => request(`/subjects/search?q=${encodeURIComponent(q)}${lang ? `&lang=${lang}` : ''}`),
  addSubject: (payload) => request('/subjects', { method: 'POST', body: payload }),
}
