import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// In dev, proxy /api to the Go backend so the browser sees same-origin
// requests (cookies flow without CORS — docs/04-api.md §CORS). The target is
// configurable so the same config works on the host (localhost:8080) and in
// docker-compose, where the backend is reachable as the `backend` service
// (VITE_PROXY_TARGET).
const proxyTarget = process.env.VITE_PROXY_TARGET || 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    // bind 0.0.0.0 so the port is reachable from outside a container
    host: true,
    port: 5173,
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
      },
    },
  },
})
