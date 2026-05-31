import { createRouter, createWebHistory } from 'vue-router'
import { api } from '../api/client'

const routes = [
  { path: '/', name: 'matchup', component: () => import('../views/MatchupView.vue') },
  { path: '/leaderboard', name: 'leaderboard', component: () => import('../views/LeaderboardView.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Client-side mirror of the access-token gate (R4/R10). UX only — the server is
// authoritative and returns 403 if ungated (docs/01-functional-spec.md S2).
router.beforeEach(async (to) => {
  if (to.name !== 'leaderboard') return true
  try {
    const me = await api.me()
    if (me.hasAccess) return true
  } catch {
    // fall through to redirect
  }
  return { name: 'matchup', query: { locked: '1' } }
})

export default router
