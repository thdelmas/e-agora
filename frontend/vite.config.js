import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// In dev, proxy /api to the Go backend so the browser sees same-origin requests
// (cookies flow without CORS — docs/04-api.md §CORS).
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
