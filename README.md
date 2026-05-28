# tickets-general

Generic event ticketing system. Multi-event, role-based admin, configurable pricing (phase × category or donation), waitlist on capacity, SES email, PayPal + bank-transfer payments. Deployed via Helm on Kubernetes.

- Implementation plan and current phase: [`docs/ROADMAP.md`](./docs/ROADMAP.md)
- Production deployment runbook: [`docs/PROD_DEPLOYMENT.md`](./docs/PROD_DEPLOYMENT.md)

## Quick start (local dev)

```sh
cp .env.example .env
make dev
```

Brings up Postgres, MinIO, mailcatcher, the Go backend, and the Next.js frontend. Ports and env vars are documented in `.env.example`.

## Layout

- `backend/` — Go API server (chi + sqlc + pgx + goose).
- `frontend/` — Next.js (App Router, TypeScript, Tailwind, next-intl).
- `deploy/` — Helm chart and k8s manifests.
- `docs/` — product spec, roadmap, production deployment runbook, and phase-specific deep-dives.
