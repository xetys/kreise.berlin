# Frontend image: Next.js 16 standalone output.
# Build context is the frontend/ directory so npm install only sees the
# frontend tree:
#   docker buildx build --platform linux/amd64 -f frontend/Dockerfile -t ... ./frontend

FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
# `npm ci` would be strictly reproducible but the lockfile drifts between
# host (Node 23) and container (Node 22) — picomatch / @swc/helpers resolve
# to slightly different versions. `npm install` auto-reconciles, which is
# the right tradeoff for MVP. Production-hardening can flip this back to
# `npm ci` once the dev / build node versions are aligned.
RUN npm install --no-audit --no-fund

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# next.config.ts bakes the rewrite destination at build time via process.env;
# pass it through so the standalone bundle proxies /api/* to the in-cluster
# backend Service. Override with --build-arg for non-Helm deployments.
ARG BACKEND_URL=http://kreise-backend:8080
ENV BACKEND_URL=$BACKEND_URL
RUN npm run build

FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

# Standalone bundle (server.js + minimal node_modules).
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public

# Cache dir Next writes to at runtime.
RUN mkdir -p .next/cache && chown -R nextjs:nodejs .next

USER nextjs
EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
CMD ["node", "server.js"]
