// Local preview config: serves mock /api responses so the public stats charts render without the Go
// backend, for eyeballing the mobile chart sizing. Run:
//   npx vite --config vite.preview.mjs
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

function mockApi() {
  return {
    name: 'mock-eagora-api',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url || ''
        if (!url.startsWith('/api/')) return next()
        res.setHeader('Content-Type', 'application/json')

        if (url.startsWith('/api/me')) {
          res.end(JSON.stringify({ hasAccess: true, canAdd: true, humanVerified: true, votes: 7 }))
          return
        }

        if (url.startsWith('/api/stats')) {
          const m = url.match(/days=(\d+)/)
          const days = Math.min(365, Math.max(1, m ? parseInt(m[1], 10) : 30))
          const today = new Date()
          const daily = []
          for (let i = days - 1; i >= 0; i--) {
            const d = new Date(today)
            d.setUTCDate(d.getUTCDate() - i)
            const t = days - 1 - i
            const wave = Math.sin(t / 6) * 0.5 + 0.5
            const trend = t / days
            const votes = Math.round(20 + wave * 60 + trend * 90 + ((t * 37) % 17))
            const visitors = Math.round(8 + wave * 22 + trend * 30 + ((t * 13) % 9))
            const voters = Math.round(votes * 0.35 + ((t * 7) % 5))
            const added = t % 4 === 0 ? (t * 3) % 4 : 0
            daily.push({ date: d.toISOString().slice(0, 10), votes, voters, visitors, added })
          }
          res.end(JSON.stringify({
            generatedAt: new Date().toISOString(),
            days,
            totals: { votes: 12840, voters: 1932, visitors: 2517, subjects: 391, userContributed: 28 },
            daily,
          }))
          return
        }

        res.end('{}')
      })
    },
  }
}

export default defineConfig({
  plugins: [vue(), mockApi()],
  server: { host: true, port: 5174 },
})
