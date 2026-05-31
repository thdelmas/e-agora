# syntax=docker/dockerfile:1
# Dev image: runs the Vite dev server (hot reload) and proxies /api to the
# backend service. The proxy target is set via VITE_PROXY_TARGET in compose
# (see vite.config.js). For a production image you'd `npm run build` and serve
# the static dist/ — out of scope for `make dev`.

FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
EXPOSE 5173
# --host binds 0.0.0.0 so the port is reachable from outside the container.
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
