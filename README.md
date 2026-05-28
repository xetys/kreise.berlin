<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.png">
    <img alt="kreise.berlin" src="docs/assets/logo-light.png" width="140" height="140">
  </picture>
</p>

<h1 align="center">kreise.berlin</h1>

<p align="center">
  A multi-event ticketing platform for conscious gatherings in Berlin — festivals, ecstatic dances, tea ceremonies, and the like. Live at <a href="https://kreise.berlin">kreise.berlin</a>.
</p>

---

## What it does

One deployment hosts many events. Each event has its own name, description, banner, color set, location, schedule, and price policy. Two pricing modes are supported per event:

- **Donation** — optional payment, suggested range, default for tea events and small circles.
- **Phase × category matrix** — early-bird/normal/late-bird × adult/reduced × day/weekend/full, with coupon support (fixed-amount, percentage, or guestlist).

Bookings move `booked → paid` once an admin confirms payment — either reconciling a bank transfer or flipping a PayPal.me / at-door booking after the money arrives. Each ticket carries a signed QR code; at the door, organizers check in **individual participants** with a phone-camera scanner. The check-in flow handles donation amounts collected at the door and matrix tickets paid on arrival, with an undo and a per-event cash summary.

When an event reaches its participant limit, further bookings land on a FIFO waitlist with self-service removal. On cancellation, donation events auto-promote the next eligible waiter; paid events broadcast a "spot opened" email and the first claimant locks the seat.

Three admin roles — `global_admin`, `event_admin`, `event_manager` — keep operator access scoped to the events they own or are assigned to. Onboarding is invite-only: an admin sends an email with a one-time setup link, the invitee picks their password, done.

All transactional email goes through Amazon SES, bilingual DE + EN.

## Tech stack

- **Backend** — Go 1.25, `chi` HTTP router, `sqlc` + `pgx/v5` for type-safe queries, `goose` migrations embedded into the binary, `argon2id` password hashing, session cookies validated against a versioned password counter for instant force-logout.
- **Frontend** — Next.js 16 (App Router) + TypeScript, Tailwind CSS v4, `next-intl` (DE default, EN sister locale, instant client-side switching).
- **Datastore** — PostgreSQL 16. **Object storage** — MinIO (S3-compatible; same code path for AWS S3 in production).
- **Email** — Amazon SES.
- **Payments** — manually reconciled: bank transfer, PayPal.me link per event, or at-door cash. No payment-gateway callbacks.
- **Deploy** — Docker images on `linux/amd64`, packaged as a Helm chart.

The roadmap and architectural decision log live in [`docs/ROADMAP.md`](docs/ROADMAP.md).

---

## Local development

### Prerequisites

- Go ≥ 1.25
- Node.js ≥ 20 (Next 16 requirement)
- Docker + Docker Compose
- `make`

### First-time setup

```sh
cp .env.example .env       # adjust DB and mail credentials as needed
make up                    # bring up Postgres, MinIO, Adminer (background)
make migrate-up            # apply all pending migrations
make seed                  # (optional) seed a reference event for poking around
```

The compose stack runs on:

| Service  | URL                                    |
| -------- | -------------------------------------- |
| Postgres | `postgres://tickets:tickets@localhost:5432/tickets` |
| MinIO    | `http://localhost:9000` (S3 API), `:9001` (console) |
| Adminer  | `http://localhost:8081`                |

### Running the servers

```sh
make dev          # docker compose up + backend + frontend, in one shell
```

Or run each piece separately in its own terminal:

```sh
make dev-backend  # Go server at http://localhost:8080
make dev-frontend # Next dev server at http://localhost:3000
```

The frontend proxies `/api/*` to the backend, so you only need to hit `http://localhost:3000` while developing.

### Stopping & resetting

```sh
make down         # stop containers (volumes preserved)
make reset-db     # drop and recreate the dev database (volumes wiped)
```

---

## Database migrations

Migrations are plain SQL files under `backend/internal/migrations/`, applied with `goose` and embedded into the server binary so production pods run them on boot.

```sh
make migrate-up                              # apply everything pending
make migrate-down                            # roll back the latest migration
make migrate-status                          # show what's applied / pending
make migrate-create NAME=add_some_column     # scaffold a new timestamped file
```

After editing or adding queries under `backend/sqlc/queries/`, regenerate the Go bindings:

```sh
make sqlc-gen     # codegen into backend/internal/db/
make sqlc-vet     # static-check queries against the schema
```

Migrations are designed to be **additive and backward-compatible** — the rolling deploy starts new pods while old ones still serve, and both must coexist during the cutover.

---

## Tests & lint

```sh
make test-db-init        # one-time: create the tickets_test DB and migrate it
make test                # backend (go test -race) + frontend
make lint                # golangci-lint + frontend lint
make vet                 # go vet
make vuln                # govulncheck against the dependency tree
```

Backend integration tests share a single database, so they run with `-p 1` — expect the suite to take a minute, not seconds.

---

## Deploying with Helm

Production builds are multi-arch `linux/amd64` Docker images pushed to a container registry, then rolled out as a single Helm release.

### Build & push the images

```sh
IMAGE_TAG=v0.14.0 make docker-push
```

This rebuilds both `backend` and `frontend` images, tags them with the supplied `IMAGE_TAG` (also re-tagging `:latest`), and pushes them to the registry configured in the `Makefile` (`IMAGE_REPO`). The `IMAGE_TAG` defaults to the current git short-SHA if unset, which is handy for ephemeral testing but never for a release — always pin a version.

### Inspect the chart before deploying

```sh
make helm-template IMAGE_TAG=v0.14.0
```

Renders all chart templates to stdout so the diff between the current and proposed manifests is visible before anything touches the cluster.

### Upgrade or install

Production secrets (signing key, DB password, MinIO creds, SES credentials) live outside the chart in a values file kept on the operator's machine — `deploy/secrets.values.yaml`, which is gitignored.

```sh
make helm-deploy \
  IMAGE_TAG=v0.14.0 \
  DEPLOY_VALUES=deploy/secrets.values.yaml
```

The target wraps `helm upgrade --install kreise ./chart --namespace kreise-berlin --create-namespace -f $DEPLOY_VALUES --set image.tag=$IMAGE_TAG`. `--install` makes it idempotent: fresh cluster or existing release, same command. Override `KUBECONFIG_PATH`, `NAMESPACE`, or `RELEASE` on the command line as needed.

After the upgrade, watch the rollout:

```sh
KUBECONFIG=$KUBECONFIG_PATH kubectl -n kreise-berlin rollout status deploy/kreise-backend  --timeout=90s
KUBECONFIG=$KUBECONFIG_PATH kubectl -n kreise-berlin rollout status deploy/kreise-frontend --timeout=90s
```

The chart includes the backend Deployment, frontend Deployment, in-cluster Postgres, in-cluster MinIO with a bucket-bootstrap Job, Ingress, Service, and a one-shot Job that seeds the first `global_admin` on a fresh install.

### Rollback

```sh
KUBECONFIG=$KUBECONFIG_PATH helm -n kreise-berlin history kreise
KUBECONFIG=$KUBECONFIG_PATH helm -n kreise-berlin rollback kreise <REV>
```

Helm restores the previous chart values and image tag but does **not** roll back database migrations. The migration policy (additive only) means a code-level rollback to the previous tag is usually safe; if a migration is genuinely destructive, restore the database from backup instead.

---

## Repository layout

```
backend/                Go API server
  cmd/server            HTTP server entry point
  cmd/migrate           goose CLI
  cmd/bootstrap-admin   first-admin seeder (runs as a Helm Job)
  internal/             chi handlers, sqlc-generated DB layer, auth, mail, …
  internal/migrations/  embedded SQL migrations
  sqlc/queries/         input for sqlc codegen
frontend/               Next.js 16 + Tailwind v4
  src/app/[locale]/     bilingual route tree (de | en)
  src/components/       shared UI (PublicShell, EventDetailLayout, scanner, …)
  messages/{de,en}.json next-intl catalogs
chart/                  Helm chart (templates/, values.yaml)
deploy/                 environment-specific overlays and helper manifests
docs/                   ROADMAP and asset files
```

---

## License

All rights reserved. Contact the maintainer before reusing.
