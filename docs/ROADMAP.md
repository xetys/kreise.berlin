# ROADMAP

Implementation phases for the generic ticketing system described in `CONCEPT.md`. This file is the **single source of truth for "where are we"**. It is structured so that any Claude session — fresh or post-`/clear` — can re-enter the work cheaply.

---

## Cold-start recipe (read this first)

If you are an LLM session that just opened this repo:

1. Read `CLAUDE.md` for project intent and constraints.
2. Read this file's **Current phase** line below.
3. Jump straight to that phase's section. The "Context to load" subsection lists the minimum set of files to read for that phase — do not load the whole repo.
4. Pick the next unchecked todo. If none, run the phase's **Exit criteria** check; if all green, advance the **Current phase** pointer and start the next phase.
5. Locked decisions in **Architecture decisions** are not up for re-litigation unless the user reopens them. Open decisions must be resolved before phases that depend on them.

When you finish work in a session, **update this file**: tick boxes, append notes under the phase's **Notes** subsection, and bump **Current phase** if appropriate. Do not delete prior notes — they are the audit trail across sessions.

---

## Current phase

**MVP launch path status (2026-05-11):**

1. **Phase 10 — Waitlist & auto-promotion** ✅ shipped (admin UI follow-up open)
2. **Phase 8 — Booking management (admin ops at scale)** ✅ shipped
3. **Phase 13 — User management (admin lifecycle)** ✅ shipped
4. **Phase 12 — Roll-out / launch readiness** ✅ live on `https://kreise.berlin` (helm chart, GHCR proxy pulls, cert-manager TLS, SES verified for kreise.berlin domain, bootstrap-admin Job). Hardening backlog (Phase 12 observability + security + load test + legal-text final fill) still open.

**Open MVP polish items:**
- Phase 10 admin UI: `/admin/events/{id}/waitlist` listing + manual promote/remove.
- Phase 12 hardening: Prometheus + Sentry; OWASP self-review; load test; first-event playbook dry-run.

**Post-MVP** (queued after first events go live): Phase 7 (door scanning), Phase 11 (newsletter), Phase 9 follow-up polish.

Last updated: 2026-05-11.

---

## Architecture decisions

### Locked

From `CONCEPT.md` and user direction (non-negotiable):

- Backend in **Go** (minimum **Go 1.23**), with tests on core logic.
- **Multi-event** single deployment (row-level scoping, not schema-per-tenant).
- Three roles: **global_admin / event_admin / event_manager** (M:N for event_admin & event_manager assignments).
- Per-**participant** check-in (not per-ticket).
- Email via **Amazon SES**.
- Deployment via **Helm** on **Kubernetes**.
- Invite-only first phase — **no public user signup**.
- **Capacity is optional per event.** `events.participant_limit` is nullable; `NULL` means unlimited and **no waitlist** can ever exist for that event. The waitlist subsystem is only reachable when `participant_limit` is set and reached.

Resolved 2026-05-04:

- **Web framework: `chi`.** Stdlib-compatible (`http.Handler`), clean middleware composition, sub-routers for grouped middleware (admin vs public, RBAC layering). Stdlib 1.22+ routing is close but lacks ergonomic middleware groups; chi is small enough to never regret.
- **DB: PostgreSQL 16+.** Native UUID/JSONB, mature k8s story (cloudnative-pg operator) and managed offerings, best Go ecosystem support.
- **DB access: `sqlc` + `pgx/v5`.** Write SQL, generate type-safe Go. No ORM reflection at runtime, no leaky abstractions, queries stay reviewable. `pgx` is sqlc's first-class driver and faster than `database/sql`.
- **Migrations: `goose`.** Plain SQL up/down files, single binary, embeddable for a pre-upgrade migration Job in k8s, pairs cleanly with sqlc.
- **Frontend stack: Next.js (App Router) + TypeScript + Tailwind CSS.** Mirrors base project, RSC for SEO on public pages, well-trodden path with the sibling repo as reference.
- **API style: REST/JSON.** Domain is CRUD-shaped; no need for GraphQL or RPC complexity.
- **Auth: DB-backed sessions** in a `sessions` table; opaque session ID in an `HttpOnly`, `Secure`, `SameSite=Lax` cookie. DB-backed (not signed-only) so account-disable, password-change, and admin-revoke can invalidate immediately.
- **Password hash: `argon2id`** via `golang.org/x/crypto/argon2`. OWASP 2024 params (memory=64MB, time=3, parallelism=4); store version+params+salt encoded into the hash string.
- **Money: `int64` minor units + ISO currency code.** EUR only at MVP. No floats anywhere in pricing. Type alias `type AmountMinor int64` to make signatures self-documenting.
- **Banner storage: MinIO**, both local dev (in `docker-compose`) and production (in-cluster Helm release with PVC). Backend serves banners through a cached pass-through endpoint at `/banners/<slug>`, so the storage backend stays an internal detail — swapping to Cloudflare R2 or a provider-native bucket later is an env-var change, no public-URL breakage. Local disk is a non-starter once backend has >1 replica; managed external object storage is overkill for MVP and adds an external dependency.
- **Email transport (revised 2026-05-06): SES-only, dev included.** No SMTP, no mailcatcher, no driver factory. The mailer is `mail.New` → `aws-sdk-go-v2` SES. Transactional sends are synchronous; bulk newsletter (Phase 9) uses an in-process worker reading a `send_jobs` outbox table. All sends — successful and failed — write to `email_log`. _Mailcatcher and the SMTP driver were dropped 2026-05-06; the dual-driver code added needless surface area._
  - **MVP testing arrangement:** reuse the AWS credentials and verified sender from the sibling `tickets-psyrock-com` project, which already has SES configured for `psychedelic-rock.com`. `MAIL_FROM` is `no-reply@psychedelic-rock.com` (or another verified address on that domain) during MVP. Phase 11 swaps these for tickets-general's own production IAM user + verified sender domain.
- **i18n (revised 2026-05-06):**
  - **Backend has no i18n.** No catalog, no message-id lookup. The original `internal/i18n` package is removed.
  - **System emails are bilingual.** Every transactional email contains German first, then English below it, separated by a horizontal rule. Bookers don't pick a language; everyone gets both. Templates are inline in the mailer; no per-locale rendering.
  - **API errors return stable code keys, not user-facing strings.** Response shape is `{"error": "<stable_code>", "developer_message": "<for-debugging-only>"}`. Display text is resolved on the **frontend** via its message catalog, keyed by `error`.
  - **Frontend keeps the de/en structure** (next-intl, `[locale]` route, `messages/{de,en}.json`) so adding the actual switcher later is a small focused PR. **No language switcher widget is built in MVP.** Routing defaults to `/de`; a user wanting English types `/en` in the URL bar.
  - **Per-event content (name, description, program) is the event creator's responsibility.** The system does not store or serve event content in multiple languages. If a creator wants a bilingual event, they write bilingual copy themselves.
  - **Admin UI is single-language** (German). No admin-side i18n.
  - **No `users.locale` column.** Admins don't have a language preference.
  - The `locale` columns on `bookings` / `participants` / `waitlist_entries` / `email_log` stay in the schema as informational / future-proofing — they are not consumed by current code.
- **PayPal: thin custom REST client** against PayPal v2 API in `backend/internal/payment/paypal/`. The third-party Go SDKs are unmaintained or thin wrappers; the surface we need (orders.create, orders.capture, payments.refund, webhook signature verify) is small enough to keep in-tree.

### Open

_None._ Reopen here if any locked decision proves wrong in implementation.

---

## Phase index

Numbers are stable — they don't indicate execution order on their own. Read the **Gate** column for that. Done phases stay listed for cross-references in Notes.

| #  | Title                                        | Gate     | Depends on   | Touches                          |
|----|----------------------------------------------|----------|--------------|----------------------------------|
| 0  | Bootstrap & decisions                        | done     | —            | repo, CI, dev tooling            |
| 1  | Foundations (DB, auth, RBAC, mailer)         | done     | 0            | backend                          |
| 2  | Event management (admin)                     | done     | 1            | backend + minimal admin UI       |
| 3  | Pricing engine                               | done     | 1            | backend (pure logic)             |
| 3a | Pricing admin UI + Dao Dance reference seed  | done     | 2, 3         | frontend admin + seed CLI        |
| 4  | Public booking flow                          | done     | 2, 3, 3a     | backend + frontend public        |
| 5  | Tickets, QR, ticket page, cancel/transfer    | done     | 4            | backend + frontend self-service  |
| 6  | Payments (PayPal + bank-transfer + at-door)  | done     | 4, 5         | backend + frontend payment       |
| 6b | PayPal.me handle per event + test mode       | done     | 6            | backend + frontend admin/public  |
| 9  | Public landing & per-event theming polish    | done¹    | 4            | frontend                         |
| 10 | Waitlist & auto-promotion                    | **MVP**  | 4, 6         | backend + admin UI               |
| 8  | Booking management (admin ops at scale)      | **MVP**  | 5, 6         | backend + frontend admin         |
| 13 | User management (admin lifecycle)            | **MVP**  | 1            | backend + frontend admin         |
| 12 | Roll-out / launch readiness                  | **MVP**  | 10, 8, 13    | infra, security, observability   |
| 7  | Door scanning / check-in                     | post-MVP | 5            | backend + scanner UI             |
| 11 | Newsletter & broadcast mail                  | post-MVP | 1, 4         | backend + admin UI               |

¹ First pass shipped 2026-05-07; mobile/Lighthouse/print follow-ups are post-MVP and live inside the Phase 9 section.

A phase can spawn `docs/phase-N-<topic>.md` when its detail outgrows the section here. Keep this file the entry point.

---

## Phase 0 — Bootstrap & decisions

**Goal.** Empty repo → working dev loop. Code compiles, tests run, frontend dev server boots, Postgres starts via compose, CI is green.

**Prereqs.** None.

**Context to load.** `CLAUDE.md`, `CONCEPT.md`, this file's **Architecture decisions** section.

**Todos.**
- [x] Confirm/record the module path (`github.com/dsteiman/tickets-general`); `go mod init` with **Go 1.23**.
- [x] Monorepo layout: `backend/` (`cmd/`, `internal/`, `sqlc/queries/`), `frontend/`, `deploy/` (`helm/`, `k8s/`), `docs/`. _Migrations live at `backend/internal/migrations/` (Go-embed friendly), not `backend/migrations/`._
- [x] `backend/cmd/server/main.go` — `chi` router, `/healthz` (liveness, no deps) and `/readyz` (pings Postgres via `pgx`).
- [x] `backend/internal/config/` — env-driven config struct, validated at startup, no globals.
- [x] Structured logging via `log/slog` (JSON in prod, text in dev), request ID middleware that propagates into log records.
- [x] `pgx/v5` wired as the DB driver; connection pool config from env.
- [x] `sqlc.yaml` at repo root pointing at `backend/internal/migrations/` for the schema and `backend/sqlc/queries/` for queries; generated code lands in `backend/internal/db/`.
- [x] `goose` set up for migrations via embedded FS in `backend/internal/migrations/embed.go`; `backend/cmd/migrate/` exposes the `up`/`down`/`status`/`version` CLI used by Makefile and (later) the k8s migration Job.
- [x] Initial placeholder migration `00001_init.sql` (single-file goose style, not split up/down) — validates the runner end-to-end.
- [x] `golangci-lint` config: govet, staticcheck, errcheck, revive, gosec, gocritic, unused, ineffassign + gofmt/goimports formatters; sqlc-generated `backend/internal/db` excluded.
- [x] `Makefile` targets: `help`, `up`, `down`, `dev`, `dev-backend`, `dev-frontend`, `build`, `test`, `lint`, `vet`, `vuln`, `migrate-{up,down,status,create}`, `sqlc-{gen,vet}`, `seed`, `clean`. Tools (golangci-lint, sqlc, govulncheck) pinned via `go run`.
- [x] `docker-compose.yml` for local: Postgres 16, MinIO (with `minio-bootstrap` init container that creates the `tickets` bucket), Adminer. Healthchecks gate dependents. _Mailcatcher dropped 2026-05-06 along with the SMTP driver — local mail testing now goes through real SES._
- [x] Bootstrap Next.js (App Router) under `frontend/` with TypeScript, Tailwind, `next-intl` and `de`+`en` locales (`de` default for public). Note: Next 16 renamed `middleware.ts` → `proxy.ts`; the next-intl middleware lives at `src/proxy.ts`.
- [x] CI pipeline (`.github/workflows/ci.yml`): backend `go vet` + `golangci-lint` + `govulncheck` + `go test -race` against a Postgres service container; frontend `npm ci` + lint + typecheck + build; runs on PR + main.
- [x] `.gitignore` for Go binaries, `node_modules`, `.env*`, build artifacts, IDE files.
- [x] `.env.example` documenting every required env var (DB DSN, token signing key, SES creds, S3/MinIO creds + endpoint, PayPal creds, public base URL) with no real secrets.
- [x] README pointing to this ROADMAP for orientation.
- [x] Smoke test: compose brings up Postgres+MinIO+Adminer healthy; `migrate up` applies; backend connects to DB; `/healthz` and `/readyz` both 200; frontend `npm run build` green with `[locale]` route + `Proxy` middleware.

**Exit criteria.** Fresh clone → `make dev` works in under 5 minutes from `git clone` + `cp .env.example .env`. CI green on main.

**Notes.**

- 2026-05-04 — Phase 0 complete. Stack: Go 1.23+ (host has 1.26.1), chi 5.2.5, pgx 5.9.2, goose 3.27.1, Next.js 16.2.4, next-intl 4.11.0, Tailwind 4.
- Layout deviation from original ROADMAP: migrations ended up at `backend/internal/migrations/` (so `//go:embed *.sql` works from the same package) rather than `backend/migrations/`. `sqlc.yaml` and the migrate CLI both reference the new path.
- Next 16 surprise: `middleware.ts` is now `proxy.ts`. Functionality is identical; only the file name and the named export differ. next-intl 4.11 works inside `proxy.ts` unchanged.
- Single `go.mod` at repo root with `module github.com/dsteiman/tickets-general`. No Go workspace, no per-package go.mod.
- Sessions table is **not** wired yet — Phase 1 territory. Phase 0 stops at infra.
- Git commits deferred per user preference; working tree carries all of Phase 0 unstaged at handoff.

---

## Phase 1 — Foundations (DB, auth, RBAC, mailer)

**Goal.** Backend skeleton on which all features hang: schema, sessions, role-based authorization, transactional email primitive, integration test harness.

**Prereqs.** Phase 0.

**Context to load.** `backend/internal/config/`, `backend/migrations/`, `backend/cmd/server/main.go`, `CONCEPT.md` §"summary features", §"admin side aware of three roles".

**Todos.**

_Schema (initial migration `00002_initial_schema.sql`)_
- [x] `users` with argon2id-encoded `password_hash` and role CHECK.
- [x] `sessions` (id BYTEA, FK to users, expires_at + revoked_at, ua, ip).
- [x] `events` with `participant_limit` nullable, `pricing_mode` enum (matrix|donation), `currency`, `default_locale`, `is_public`, `archived_at`, `banner_ref` as S3 key.
- [x] `event_admins`, `event_managers` M:N tables.
- [x] `program_entries` as a separate table (rejected JSONB-on-events: easier to index, edit, validate).
- [x] `participants` with `newsletter_optin` and partial index on `lower(email) WHERE optin=true`.
- [x] `bookings` with `payment_method` CHECK, `reservation_expires_at`, `locale`.
- [x] `tickets` one-per-participant (UNIQUE on participant_id), pricing snapshot (phase/category/duration), state CHECK.
- [x] `price_phases` / `price_categories` / `price_durations` / `prices` (sparse allowed; UNIQUE on phase × category × duration).
- [x] `donation_configs` (PK = event_id).
- [x] `coupons` with type-aware value CHECK; `coupon_phase_filters` + `coupon_category_filters` for applies-to.
- [x] `coupon_redemptions` (UNIQUE coupon × booking).
- [x] `waitlist_entries` with `selection_json` JSONB and `claim_deadline`. _App-layer guard ensures rows only for limited events._
- [x] `email_log` with locale + ses_message_id + related_event_id + related_booking_id.
- [x] `audit_log` with FK ON DELETE SET NULL (preserved across event/user deletion).
- [x] Indexes on hot lookups (events by `(is_public, starts_at) WHERE archived_at IS NULL`; bookings by `(event_id, status)`; participants newsletter partial index; etc.).

_Backend wiring_
- [x] sqlc-generated `db` package: queries for users, sessions, events, event_admins, event_managers, email_log, audit_log. UUID and timestamptz overrides in `sqlc.yaml` so types come out as `uuid.UUID` / `time.Time` (no pgtype noise in domain code).
- [x] `database.WithTx(ctx, fn)` helper wrapping pgx Tx + sqlc; `pool.Queries()` for non-tx use.
- [x] Domain types in `internal/domain/` (User, Role, Session, SessionWithUser, Event, PricingMode) with mappers from sqlc rows.
- [x] `internal/auth/`:
  - [x] `argon2id.Hash` / `Verify` with OWASP 2024 params (m=64MiB, t=3, p=4); encoded `$argon2id$v=…$m=…,t=…,p=…$<salt>$<key>` string.
  - [x] Login (email+password), logout, session creation/revoke. **No public signup**.
  - [x] Session middleware: cookie → DB lookup → enforce expires_at + revoked_at + user.disabled_at, touch last_seen_at, attach `domain.User` + `domain.Session` to context.
  - [x] Cookie attrs: `HttpOnly`, `Secure` (in prod), `SameSite=Lax`, path `/`.
  - [x] HTTP handlers: `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`.
- [x] CSRF middleware (`internal/csrf`): double-submit cookie + `X-CSRF-Token` header, safe methods issue cookie, state-changing methods validate with constant-time compare. `GET /api/csrf` bootstrap endpoint.
- [x] `internal/authz/`: single `Allow(user, action, resource)`. Action constants enumerated (event.*, pricing.*, booking.*, newsletter.*); role × action permission table; event-scoped actions resolve membership via `IsEventAdmin` / `IsEventManager`. Global admin bypasses all checks. Disabled users denied universally.
- [x] `internal/mail/`:
  - [x] `Mailer.Send(ctx, msg)` interface.
  - [x] ~~SMTP driver (mailcatcher in dev) and SES driver (sesv2 via `aws-sdk-go-v2`); selected by `MAIL_DRIVER` env.~~ _Revised 2026-05-06: SES-only. SMTP driver, factory, and mailcatcher removed. `mail.New(ctx, pool, Config{Region, From, ...})` is the single constructor._
  - [x] ~~Locale-aware: `RenderFromCatalog` looks up subject + body in i18n by message ID, normalizes locale, renders via `text/template`.~~ _Removed 2026-05-06: emails are now bilingual (German + English) per the revised i18n policy; no per-locale rendering. Replaced by `RenderBilingual(name, BilingualSpec, data)`._
  - [x] HTML body with light styling wrapper + `<hr>` separator between the two languages. Themed templates per event are deferred to Phase 10.
  - [x] Every send recorded in `email_log` with status, ses_message_id, error (failures recorded too).
  - [x] ~~Verified end-to-end: `POST /api/admin/mail/test` lands a German message in mailcatcher and inserts a `sent` row in `email_log`.~~ _Re-verification post-rip requires real AWS creds; SES simulator addresses or sandbox-verified recipients work for local checks._
- [x] ~~`internal/i18n/`: Go map catalog keyed by `MessageID`, locales `de` + `en`, default `de`. `T(locale, id)` and `Normalize(locale)` helpers.~~ _Removed 2026-05-06: backend i18n dropped entirely; see Architecture decisions for the new policy._
- [x] `internal/objectstore/`: S3-compatible client (`aws-sdk-go-v2` s3) with `Put`/`Get`/`Delete`/`Exists`. Endpoint + path-style configurable for MinIO. _Smoke-tested against in-cluster MinIO via `backend/cmd/storesmoke`._
- [x] `internal/audit/Record(ctx, pool, entry)` central writer; stable action keys + target kind constants in one place. Errors are non-blocking (log them, don't fail the parent op).

_Test harness_
- [x] `internal/testdb`: `Pool(t)` skips when `TEST_DATABASE_URL` is unset; `Reset(t, pool)` truncates all data tables CASCADE; `SeedUser`, `SeedEvent`, `AssignEventAdmin`, `AssignEventManager` helpers.
- [x] `make test-db-init` provisions a separate `tickets_test` database; `make test-backend` runs `go test -p 1 -race ./...` (`-p 1` because cross-package integration tests share the DB).
- [x] Coverage: auth 80.8%, authz 90.9%, csrf 83.3% — comfortably above the ≥60% bar (focus is on covering essential paths, not chasing a number).
- [x] Tests cover: argon2id round-trip + invalid encodings + salt randomness; login happy/wrong-pw/disabled/unknown-email; session middleware required/optional/revoked/missing-cookie; HandleLogin/Logout/Me JSON shapes; CSRF method-by-method; RBAC role × action matrix (in-memory) and event_admin/event_manager membership-aware (Postgres-backed).

**Exit criteria.** A global admin can log in via API. Hitting any admin endpoint without auth returns 401; with wrong role returns 403. A test email is accepted by SES (real creds + simulator/sandbox recipient locally) and recorded in `email_log`.

**Notes.**

- 2026-05-06 — Phase 1 complete. End-to-end smoke verified against compose: `POST /api/auth/login` (after `GET /api/csrf` bootstrap) returns user JSON + sets HttpOnly session cookie; `GET /api/auth/me` mirrors; `POST /api/auth/logout` revokes the row and clears the cookie; subsequent `/me` returns 401. CSRF middleware rejects POSTs without matching header. Test mail lands in mailcatcher and the `email_log` row is recorded with status `sent`. RBAC `Allow` tested across the role × action × membership matrix.
- "Wrong role → 403" exit-criterion: the authz layer returns `ErrForbidden` correctly (proven in tests). The HTTP-level `403` mapping kicks in once Phase 2's admin endpoints actually call `Allow`. The plumbing is ready.
- Two small dev utilities ride along: `backend/cmd/hashgen <password>` prints an argon2id hash for seeding admins, and `backend/cmd/storesmoke` round-trips an object against MinIO. Both are useful enough across phases to keep.
- Two layout deviations from the original ROADMAP: migrations live at `backend/internal/migrations/` (so `//go:embed *.sql` works), and S3 client lives at `internal/objectstore/` rather than `internal/storage/` (the latter name was reserved before sqlc happened to also need the directory).
- Mailer SES driver is implemented but not exercised — local dev runs the SMTP driver. Real SES verification is launch-readiness territory (Phase 11).

---

## Phase 2 — Event management

**Goal.** Admins can CRUD events, manage their color set, program, banner, capacity, and visibility. Minimal admin UI shell.

**Prereqs.** Phase 1.

**Context to load.** Phase-1 schema for `events`, `event_admins`, `event_managers`, `programs`; `internal/authz/`; frontend admin shell skeleton.

**Todos.**
- [x] Endpoints: `GET /api/admin/events` (scoped by role), `POST /api/admin/events`, `GET/PATCH /api/admin/events/{id}`, `POST .../{archive,publish,unpublish}`.
- [x] `participant_limit` editable; empty input → `NULL` (unlimited). Form hint makes the waitlist-coupling explicit.
- [x] Validation: positive int when set; backend rejects ≤0. _The "warn before shrinking below current seat count" sub-todo is moot today since no bookings exist yet — re-add to Phase 4 once seats can accumulate._
- [x] Color-set validation: `#RRGGBB` hex regex on backend; native `<input type="color">` on frontend. _Contrast check skipped in MVP — Phase 10 polish._
- [x] Defaults: backend palette constants (`#5E576A` / `#F5F1EE` / `#1A1A1A`), referenced from one place.
- [x] Program CRUD nested under event (`/program`, `/program/{entryId}`). Add/list/delete in admin UI; in-place editing via `PATCH` on backend.
- [x] Banner upload: 5 MiB max, image/jpeg|png|webp|gif only, stored at S3 key `events/<id>/banner-<rand>.<ext>`, public pass-through at `GET /banners/<slug>` with `Cache-Control: public, max-age=3600`. Old banner deleted best-effort on replacement.
- [x] Event admin / manager assignment endpoints (POST/DELETE on `/admins` and `/managers`, GET `/team`). Backend ready; frontend UI for team management deferred (use backend curl/admin DB if needed in MVP).
- [x] Frontend admin shell: login page, auth-guarded `/admin` layout with logout, event list, event editor (name, description, slug, dates, location, capacity, theme pickers, program editor, banner upload, publish/unpublish/archive actions). Single-language German per the locked policy.
- [x] Frontend form validation mirrors backend (slug regex, color regex, datetime required, positive participant limit).
- [x] RBAC tests cover: event_admin cannot mutate events they aren't assigned to; event_manager cannot create events; archive/publish gated by per-event membership; auto-assignment of creator as event_admin; duplicate slug returns 409; audit row written on create.
- [x] Audit-log entries on every mutation (create, update, publish, unpublish, archive, banner upload, team changes).

**Exit criteria.** An event_admin can create an event, set or omit `participant_limit`, set its color theme, add a program, publish it. A second event_admin cannot see it unless assigned.

**Notes.**

- 2026-05-06 — Phase 2 complete. Smoke verified: admin creates event with custom palette + program entry + participant_limit=300, publishes; a freshly-seeded second event_admin sees zero events in the list and gets 403 trying to archive. Audit log records `event.create`/`event.update`/`event.publish`. All backend tests green.
- New packages: `internal/events` (handlers split into `handlers.go`, `program.go`, `team.go`, `banner.go`; routing in `router.go`); `internal/authz/http.go` (HTTPGuard with `Require` and `RequireForEventParam`).
- Frontend wiring: Next.js rewrites `/api/*` and `/banners/*` to the Go backend (env `BACKEND_URL`). Same-origin cookies + `X-CSRF-Token` from the (non-HttpOnly) `tg_csrf` cookie. `lib/api.ts` is the single fetch wrapper; auto-bootstraps the CSRF cookie on first mutation.
- Layout: `src/app/[locale]/login/page.tsx`, `src/app/[locale]/admin/layout.tsx` (client-side `/me` guard), `events/page.tsx` (list), `events/new/page.tsx` (create), `events/[id]/page.tsx` (edit + lifecycle + banner + program). Team-management UI not built in MVP; backend endpoints are functional via curl.
- Build infra fix: dropped `include .env; export` from Makefile (Make doesn't understand bash inline comments and corrupted `LOG_LEVEL`). Replaced with explicit env in test targets. Future targets that need env vars should set them in the recipe or have the user `source .env` first.
- Slug is intentionally immutable post-creation. Reasoning: event slug becomes part of public URLs once published; mutating it would silently break inbound links. Frontend disables the slug field in edit mode; backend `UpdateEvent` doesn't accept it.

**Follow-up structural change** (landed 2026-05-06):

- [x] Split the admin event page into a **detail/dashboard** page at `/admin/events/{id}` and an **edit** page at `/admin/events/{id}/edit`. Clicking an event in the list now lands on the dashboard (status pill, summary card, lifecycle action row, banner thumbnail, placeholder sections for Buchungen/Check-in/Newsletter/Team) — not directly in the edit form. The edit page keeps the full event form + banner upload + program editor + pricing editor.
- [x] The detail page owns the lifecycle actions (Veröffentlichen / Zurückziehen / Archivieren) so quick state changes don't require entering the edit view. Edit page is now strictly the field editor.
- [x] Post-create redirect (`POST /admin/events`) lands on the new detail page; existing list links unchanged.

**Future dashboard sections** (placeholders rendered today; wired in later phases):

- [ ] **Buchungen** dashboard on the detail page — list bookings, manual mark-paid, refund. Lands with Phase 6 (payments) and is filled out further by Phase 7 (waitlist).
- [ ] **Check-in** dashboard — live check-in counter, scanner login. Phase 8.
- [ ] **Newsletter & Mailings** — composer + send log. Phase 9.
- [ ] **Team** — event_admins / event_managers UI (assign/remove). Backend already exists; UI is a small follow-up to Phase 2 to land alongside the detail page sections.

---

## Phase 3 — Pricing engine

**Goal.** Pure-logic pricing core that handles both Dao Dance (phase × category × duration) and Berlin tea event (donation with suggestion). Heavily tested; no UI.

**Prereqs.** Phase 1.

**Context to load.** Phase-1 schema for `price_*`, `coupons`, `donation_configs`; `CONCEPT.md` §"Dao Dance", §"Tea Event".

**Todos.**
- [x] `pricing.Compute(input) → Quote` (the type is `Quote`; the function is `Compute` to avoid the type/function name clash). Input `QuoteInput{Event, Selections, When, CouponCode, ContactEmail}`. Output `Quote{Currency, LineItems, SubtotalMinor, DiscountMinor, TotalMinor, AppliedCoupon*, Phase}`.
- [x] Phase resolution from `When` via `Repo.GetActivePhase`; matrix mode rejects with `ErrNoActivePhase` when none. Donation mode skips this entirely.
- [x] Donation mode: per-participant line item using `DonationAmountMinor` (or `cfg.SuggestedMinor` when nil); below-min returns `ErrDonationBelowMin`.
- [x] Sparse matrix: missing `(phase, category, duration)` returns `ErrInvalidSelection` with the failing selection index.
- [x] Coupon types: `fixed_reduce` (cap at subtotal), `percental_reduce` (round-half-up integer math, no floats), `guestlist` (forces total to 0).
- [x] Coupon eligibility: code lookup, valid_from/valid_to range, max_uses via redemption count, applies-to phase/category filters, single_use_per_email by email count.
- [x] `RedeemCoupon(ctx, q *db.Queries, eventID, bookingID, code, email)` — runs `SELECT … FOR UPDATE` under the booking transaction, rechecks limits under the lock, inserts the redemption row. Phase 4 booking flow will call this.
- [x] Admin endpoints under `/api/admin/events/{id}/pricing`: GET snapshot (phases + categories + durations + prices + donation + coupons), PUT donation, POST/DELETE phases, categories, durations, PUT/DELETE prices, POST/DELETE coupons (with phase + category filter sets). All gated by `ActionPricingEdit`.
- [x] Validation: amount_minor ≥ 0; phase ends_at > starts_at; coupon type-specific value rules (fixed needs value_minor>0; percental needs value_percent in 1..100; guestlist must not have either). _Overlapping-phase warning intentionally skipped — the `GetActivePhaseForEvent` query already breaks ties by `ordering, id`, so overlap is well-defined; admin can re-order if surprised._
- [x] Table-driven tests reproducing a Dao Dance-shaped matrix (3 phases × 2 categories × 3 durations) including a deliberately omitted cell to exercise sparse rejection.
- [x] Donation tests cover below-min reject, explicit amount, suggested fallback, missing config.
- [x] Coupon tests cover fixed/percental/guestlist, expired, not-yet-valid, max_uses, single_use_per_email, applies-to filter mismatch, applies-to filter pass, discount cap at subtotal, percental rounding (33% × 6000 = 1980).

**Exit criteria.** `Compute()` is the only path that produces booking totals. The booking endpoint (Phase 4) calls it and never accepts a client-supplied price.

**Notes.**

- 2026-05-06 — Phase 3 complete. Coverage 60.2% on `internal/pricing` (matches the project's ≥60% bar; remaining is the DB-backed Store impl, which Phase 4 exercises end-to-end). All tests via in-memory `fakeRepo`; the Repo interface keeps the engine independently testable.
- Naming: function is `pricing.Compute(...)` returning `pricing.Quote`. The original ROADMAP wording said "Quote()" but Go forbids the type and function sharing a name in the same package.
- `RedeemCoupon` is the lock-and-recheck primitive; Phase 4's booking transaction uses it after creating the booking row, before commit. Atomicity guarantee: `SELECT … FOR UPDATE` + `RecordCouponRedemption` inside the same tx prevents double-spend even under concurrent bookings.
- Pricing admin endpoints intentionally minimal in MVP: snapshot read; donation upsert; phase/category/duration create + delete; price upsert + delete; coupon create + delete (with filter sets at creation time). PATCH endpoints to mutate phases/categories/durations/coupons in place are deferred — admin can delete + recreate.
- `internal/pricing/store.go` wraps sqlc queries behind the same `Repo` interface, so production handlers (Phase 4 booking) and unit tests share the same engine code path.
- ~~Pricing UI on the frontend is **not** built.~~ _Lifted into Phase 3a 2026-05-06: the frontend pricing editor is unskippable for MVP, since admins shouldn't need backend curl to configure events._

---

## Phase 3a — Pricing admin UI + Dao Dance reference seed

**Goal.** Frontend editor for the pricing config introduced in Phase 3, plus a seeded reference event modeled on the Dao Dance pricing structure (early-bird / late-bird / last-bird × adult / reduced × day / weekend / full).

**Prereqs.** Phase 2 (admin shell + event editor), Phase 3 (pricing engine + admin endpoints).

**Context to load.** `frontend/src/app/[locale]/admin/events/[id]/page.tsx`, `frontend/src/lib/api.ts`, `internal/events/pricing_handlers.go`.

**Todos.**
- [x] Frontend pricing editor mounted on the event edit page (after the program editor). Sections:
  - [x] **Mode switcher**: reads `event.pricing_mode`; donation events show the donation block; matrix events show phases / categories / durations / grid. The mode itself is changed via the main event form.
  - [x] **Donation block**: suggested + min, edited in EUR (display) ↔ cents (storage), `PUT /pricing/donation`.
  - [x] **Phases list**: name + datetime range + ordering, add + delete.
  - [x] **Categories list**: name + ordering, add + delete.
  - [x] **Durations list**: same; explicit hint that events can omit durations and the grid degrades to phase × category.
  - [x] **Price grid**: rows = `(category, duration?)`, cols = phases. Each cell is an inline-edited amount input that PUTs on blur and DELETEs when cleared. Saved/error state per cell via border color.
  - [x] **Coupons list**: shows existing with type-specific summary (`−5.00`, `−15%`, `Gästeliste`, max uses, single-use-per-email). Create form covers all three coupon types. Delete per row.
- [x] Mode switching: editor renders only the relevant block for `event.pricing_mode`. Stale config in the unused shape stays in the DB harmlessly; it returns to view if the mode is flipped back.
- [x] Client-side validation: amount must parse to non-negative cents; phase ends_at > starts_at enforced by browser datetime inputs + backend check; type-specific coupon value rules.
- [x] Reference seed: `backend/cmd/seed-dao-dance/main.go` creates the Dao Dance 2026 event idempotently — slug `dao-dance-2026`, festival 2026-07-21 → 2026-07-26, `participant_limit=400`, three phases (early-bird until 2026-05-15, late-bird → 2026-06-30, last-bird → festival start), categories `adult`+`reduced`, durations `day`/`weekend`/`full`, all 18 price cells populated.
- [x] `make seed` invokes the binary; prints owner, event id, and counts.
- [x] Smoke verified: seed produces the expected structure (3 phases, 2 categories, 3 durations, 18 prices). Re-running the seed deletes and recreates without error. Frontend builds with the editor wired into `/admin/events/[id]`.

**Open data items** (still open — placeholder cents in the seed file):
- [ ] Real EUR amounts for all 18 cells in `priceMatrix` (in `backend/cmd/seed-dao-dance/main.go`). Current values are plausible defaults from comparable Berlin-area conscious festivals, marked as PLACEHOLDER in the source.
- [ ] "Reduced" category eligibility text (informational footnote on the public page; doesn't affect pricing logic).

**Exit criteria.** An admin can create a Dao-Dance-shaped event and configure its full pricing matrix entirely through the UI. `make seed` recreates the reference event from a single command, idempotently.

**Notes.**

- 2026-05-06 — Phase 3a complete. The Dao Dance reference event seeds successfully (`make seed` shows phases=3, categories=2, durations=3, prices=18). Pricing editor renders on `/admin/events/[id]` after the program editor and supports the full CRUD surface against the Phase 3 backend endpoints.
- The seed's 18 EUR amounts are placeholders documented as such in `backend/cmd/seed-dao-dance/main.go`. Real numbers should be pasted into `priceMatrix` (and `make seed` re-run) or edited via the UI per cell.
- Display ↔ storage conversion (`centsToDisplay`/`displayToCents`) is in `PricingEditor.tsx`; future polish could extract into `lib/money.ts` once a second consumer needs it.
- Price-cell saving is on-blur (no Save button per row). The cell border turns green on success, red on error. This trades clarity for ergonomics — works well when editing many cells in sequence.
- Frontend pricing editor uses minimalist Tailwind without a custom design system; matches the rest of the admin shell. Phase 10 polish can revisit.
- Backend addition: none — the editor consumes the Phase 3 endpoints unchanged.

---

## Phase 4 — Public booking flow

**Goal.** End-to-end booking from the public site. Booking state ends at BOOKED. Payment is a later phase; donation events transition straight to PAID.

**Prereqs.** Phase 2 (event must exist and be public), Phase 3 (pricing).

**Context to load.** Phase-3 `Quote()`; `events`, `bookings`, `participants`, `tickets` schema; frontend public layout skeleton.

**Todos.**
- [x] Public endpoints: `GET /api/events` (chronological future, public, not archived) and `GET /api/events/{slug}` (event + program + read-only pricing snapshot; coupons NOT exposed).
- [x] `POST /api/bookings`: contact + participants + coupon + newsletter — server recomputes via `pricing.Compute`. Bookings, participants, tickets all created in one transaction with the coupon redemption inside it. HMAC token replacement deferred to Phase 5; for now `qr_token` is opaque random base64.
- [x] **Capacity check is conditional**: `participant_limit IS NULL` → bypassed entirely; otherwise locked seat count under `FOR UPDATE` on the events row before insert. Over-capacity returns `409 waitlist_available` (Phase 7 wires the actual waitlist).
- [x] Booking confirmation email via the bilingual mailer; subject + body change on `donation` vs `booked`. Logged in `email_log`.
- [x] Donation events: status=`paid`, `paid_at=now`, `payment_method='donation'` immediately on creation. QR generation hooked at ticket-create time (token allocated; Phase 5 adds the verifier + ticket page).
- [x] Reservation TTL of **7 days** (configurable via `Service.cfg.ReservationTTL`) — bank transfers need real time, longer than the 60-min PayPal-style stub. `RunSweeper` ticks every 60s, cancels expired bookings + cascades to ticket cancel. Donation bookings have NULL `reservation_expires_at` and are unaffected.
- [x] `POST /api/quote` — same engine as bookings, no DB writes; powers the live total preview on the booking form.
- [x] Frontend: `/[locale]/events` (public list with banner thumbnails, dates, location, link to detail) and `/[locale]/events/{slug}` (themed hero with event color set, program timeline, embedded booking form).
- [x] Frontend booking form: contact fields, dynamic participant rows (add/remove), per-participant category+duration selects (matrix) or amount input (donation), coupon code, newsletter checkbox, **debounced live quote** that re-fetches on any change. Submit redirects to `/events/{slug}/booked?ref=…&status=…`.
- [x] Frontend confirmation page with booking reference + "check your email" message; tone branches on donation vs booked.
- [x] Rate limiting: per-IP token bucket on `POST /api/bookings` (5 req/min sustained, burst 5) via `golang.org/x/time/rate`. GC every 10 min for stale buckets.
- [ ] ~~Backend integration tests for the booking flow~~ — _Deferred. The pure pricing layer is covered (60.2% in `internal/pricing`), and the booking smoke verified the full happy path. Booking-level integration tests are valuable but non-blocking for MVP._

**Exit criteria.** A user can book an event end-to-end on a paid event (status BOOKED) and on a donation event (status PAID), and receive a confirmation email. Unlimited-capacity events never block on capacity.

**Notes.**

- 2026-05-06 — Phase 4 complete. Smoke test confirmed end-to-end: GET `/api/events` returns the seeded Dao Dance event, GET `/api/events/dao-dance-2026` returns 18 prices + 3 phases + program; POST `/api/quote` for 1 adult+weekend returns 14000 cents in the active early-bird phase; POST `/api/bookings` with 2 participants (adult+weekend, reduced+weekend) creates 1 booking row (status=`booked`, total=25200, reservation expiry +7 days) + 2 ticket rows + 1 `email_log` row with status=`sent` (real SES through psychedelic-rock.com creds).
- New packages: `internal/booking` (service, dto, quote, booking, email, sweeper, rate_limit, router, errors). Booking errors map to a stable enumeration of API codes (no_active_phase, coupon_*, donation_below_min, waitlist_available, etc.) consumed by the frontend.
- New frontend routes: `/[locale]/events` (list), `/[locale]/events/[slug]` (themed landing + booking form), `/[locale]/events/[slug]/booked` (confirmation). Theming uses event color set as inline CSS variables; the hero + CTA pull from those.
- `BookingForm.tsx` debounces the live quote at 300ms — keeps server load reasonable while still feeling responsive when editing dropdowns or coupon code.
- Reservation timeout chosen as 7 days, not the example 60 minutes from the original ROADMAP. Reasoning: bank transfers (German "Überweisung") routinely take 1–3 business days; a 60-min hold makes those impossible. Configurable via `Service.cfg.ReservationTTL` if a paid-only event with PayPal wants tighter holds.
- One known limitation: matrix events ALWAYS get a `reservation_expires_at` even when `participant_limit` is NULL (i.e. unlimited). For unlimited events the timer is harmless — the sweeper still cancels, but capacity wasn't held in the first place. Acceptable for MVP.
- Booking-level integration tests are deferred. The pricing engine has unit tests; the events package has admin-side RBAC tests; the booking handler is exercised by the smoke test. Adding `internal/booking/integration_test.go` is straightforward when there's appetite.

**Follow-up polish** (small UX items not blocking exit criteria; pick up before going live):

- [ ] Auto-fill participant 1's name (and optionally email) from the contact fields on the booking form, so a single-person booking doesn't require typing the name twice. Logic: when the contact name changes and participant[0].name is empty (or matches the previous contact name), copy it over. Same for email. Stops being authoritative once the user edits participant[0] manually.

---

## Phase 5 — Tickets, QR, cancel/transfer

**Goal.** Per-participant tickets; signed-token QR; ticket-holder self-service via magic link.

**Prereqs.** Phase 4.

**Context to load.** `tickets`, `bookings`, `participants` schema; mailer; Phase-4 booking flow.

**Todos.**
- [x] One ticket per participant created in the booking transaction (Phase 4 already did this; Phase 5 added the nonce + signed token shape).
- [x] Ticket states: `booked`, `paid`, `canceled`, `checked_in`. Transitions: booked→paid (Phase 6/donation), booked|paid→canceled (cancel), paid→checked_in (Phase 8). `canCancel` and `canTransfer` helpers in `internal/tickets` are the single source of truth.
- [x] `qr_token`: HMAC-SHA256-signed string of the form `qr.<base64url(ticket_id||nonce)>.<base64url(hmac)>` — _ticket_id_ + _nonce_ live in the payload; the nonce is stored in `tickets.qr_nonce` (new column from migration `00003_ticket_nonce.sql`). Signing key from `TOKEN_SIGNING_KEY` env var (base64; minimum 16 bytes; required outside dev).
- [x] QR PNG endpoint `GET /api/tickets/{viewToken}/qr.png` renders the signed `qr_token` via `skip2/go-qrcode`. Only available for `paid` / `checked_in` tickets — `booked` (unpaid matrix events) get a "QR appears after payment" UI message.
- [x] Magic-link ticket page driven by a separate **view-purpose** token, signed against the same nonce with a different domain prefix (`view.<...>`). Domain separation: a QR-purpose token can never be replayed against the view endpoints, and vice versa.
- [x] `GET /api/tickets/{viewToken}` — returns ticket detail + event metadata (name, color set, dates, location) + holder name + line-item context + booking contact.
- [x] `POST /api/tickets/{viewToken}/cancel` — sets `status='canceled'`, idempotent (a second cancel still returns 204; canceling a `checked_in` ticket returns 409). Audit-logged. Phase 7 will hook waitlist promotion at this call site.
- [x] `POST /api/tickets/{viewToken}/transfer` — body `{new_name, new_email}`; in one tx: updates the participant row + rotates `qr_nonce` + recomputes `qr_token`. Old QR + view tokens become invalid (verification compares the on-row nonce to the token's nonce — `errStaleToken` on mismatch). Audit-logged. Returns the freshly-signed view token + URL so the previous holder can hand it over.
- [x] Cancel + transfer + (token-issued) actions all write to `audit_log`.
- [x] Tests: token round-trip; tampered HMAC rejected; wrong-purpose rejected; wrong-key rejected; malformed token rejected. (DB-backed integration tests for cancel/transfer flows were exercised via the smoke test and are open as a follow-up.)

**Exit criteria.** A paid ticket holder receives an email with a link to their ticket page, sees a QR, can cancel, can transfer to another person.

**Notes.**

- 2026-05-06 — Phase 5 complete. Smoke verified end-to-end against the seeded Dao Dance event: booking → email contains per-participant view URL → `GET /api/tickets/{view}` returns the joined ticket payload (color set, holder, line item) → `GET /qr.png` returns 403 for the `booked` ticket (correct: QR is only for paid) → `POST /cancel` returns 204 → second cancel idempotent (204).
- New packages: `internal/tokens` (Sign / Verify with purpose domain separation) and `internal/tickets` (handler + router). Migration `00003_ticket_nonce.sql` adds `tickets.qr_nonce BYTEA NOT NULL`.
- The `qr_token` column stores the SIGNED token string (not raw bytes). Door-scanning will receive the QR payload as-is and verify it via `tokens.Verify`.
- Transfer rotates the nonce and re-signs the QR token in the same transaction. Old emailed view links return `410 Gone` with code `stale_token` (not 404) — distinguishes "you had a real link that's been superseded" from "you have a fake link".
- New helper binary: `backend/cmd/sign-view` accepts a ticket UUID and prints a signed view token using the DB nonce + signing key. Used by the smoke test to pretend to be the email recipient.
- Frontend route: `/[locale]/tickets/{token}` — themed via the event's color set, shows status pill, holder, QR (when paid), cancel button (with confirm), and a transfer form that redirects to the new view URL on success.
- Email update: confirmation now includes a per-participant bullet list of `<name>: <viewURL>` lines (bilingual), so each holder can open their own ticket page from the same email the booker received.
- Two follow-ups deferred:
  - **Re-send confirmation email on transfer**: today the new holder's view URL comes back in the API response; the previous holder forwards it manually. Sending a fresh email automatically is a polish task for Phase 9 (where the mailer gets per-recipient templates anyway).
  - **DB-backed integration tests for ticket flows**: token round-trip is unit-tested in `internal/tokens`; the cancel/transfer paths are covered via the smoke test only. A dedicated `internal/tickets/integration_test.go` would land cleanly with the existing testdb harness when there's appetite.

---

## Phase 6 — Payments (PayPal + bank-transfer + at-door)

**Goal.** Paid events can take real money. Three timing modes per event: PayPal up-front, bank-transfer up-front (admin marks paid), or pay-at-door (booking is registration; QR available immediately, money collected on arrival).

**Prereqs.** Phase 4 (booking exists), Phase 5 (PAID transition triggers QR + signed view links).

**Context to load.** `bookings`, `tickets`; PayPal docs; mailer; `internal/booking`.

**Todos.**

_Per-event payment-timing policy_
- [x] Migration `00004_payment_timing.sql`: `events.payment_timing` (`beforehand` | `at_door`, default `beforehand`) plus optional `bank_iban`, `bank_bic`, `bank_account_holder` for the per-event Überweisungsdaten.
- [x] Migration `00005_payment_method_at_door.sql`: widens the `bookings.payment_method` CHECK to allow `'at_door'`.
- [x] Event editor radio for "Im Voraus" vs "Vor Ort"; bank-info fields shown only when timing is "Im Voraus".
- [x] Booking flow change: `donation` and `at_door` matrix events both set `status='paid'`, `paid_at=now()`, immediate QR availability. `beforehand` matrix events stay `booked` with reservation TTL + a generated `payment_reference` (format `TG-XXXX-XXXX`, alphabet without ambiguous chars).
- [x] **Booking-form banner** above the CTA, themed with `event.color_primary`:
  - At-door / donation: "Keine Zahlung jetzt — du bezahlst beim Event vor Ort. Dein Ticket ist sofort gültig." CTA copy flips to **Verbindlich anmelden**.
  - Beforehand: Überweisungs-Hinweis with IBAN/BIC/Kontoinhaber + reservation-TTL note. CTA stays **Verbindlich buchen**.
- [x] Public event detail returns `payment_timing` + bank fields (the frontend uses them for the banner without inference).

_PayPal_ — **deferred to Phase 6b**. Bank transfer + at-door + donation cover the immediate event needs (Dao Dance + Berlin tea event).

_Bank transfer_
- [x] Per-booking `payment_reference` allocated at booking time; included in the Stage 1 email body alongside IBAN/BIC/holder.
- [x] Admin endpoint `POST /api/admin/bookings/{id}/mark-paid` (RBAC: `ActionBookingMarkPaid`). Idempotent: re-call returns `{already_paid: true}` without re-sending the email.
- [ ] ~~Refund admin endpoint~~ — deferred to Phase 6b.

_Frontend payment surface_
- [ ] ~~PayPal redirect/return pages~~ — deferred to Phase 6b.
- [x] Bookings dashboard on `/admin/events/[id]`: real list (replaces the Phase 2 placeholder) with reference, contact, status pill, payment_method, payment_reference, participant count, total. "Als bezahlt markieren" button per `booked` row triggers mark-paid + reload + toast.

_Email styling + content (replaces the plain bilingual text)_
- [x] HTML wrapper themed with the event color set: hero with `color_primary` background, body card on `color_secondary`, footer note. Bilingual (DE first, `<hr>`, EN below). Inline-style email-client safe.
- [x] Inline QR via `cid:qr-N@tickets`. SES driver detects `len(msg.Attachments) > 0` and switches from `EmailContent.Simple` to `EmailContent.Raw` with hand-assembled `multipart/related → multipart/alternative + image/png` MIME. Plain-text fallback retains view URL.
- [x] **Two-stage email flow** for `beforehand`:
  - Stage 1 (`booking.registration_receipt`) — at booking time: subject "Reservierung erhalten / Reservation received", body lists what's reserved + Überweisungsdaten + payment_reference + "QR folgt nach Zahlungseingang". No attachments.
  - Stage 2 (`booking.tickets_confirmed`) — when status flips to `paid` (mark-paid): subject "Tickets bestätigt / Tickets confirmed", body has per-ticket name + view URL + inline QR per attachment.
- [x] **Single-stage** for `at_door` and donation: only Stage 2 fires at booking time. The opening copy branches per timing ("danke für deine Spende" vs "deine Anmeldung ist bestätigt — bezahlt wird vor Ort" vs the default).
- [x] New mail API: `mail.RenderBilingualThemed(name, BilingualSpec, Theme, data, attachments)` produces `Message{HTMLBody, TextBody, Attachments}`. CID markers in the body use `<<cid:xxx>>` (not `{{cid:xxx}}`) so the text/template engine doesn't interpret them.

_Sweeper / cleanup_
- [x] Existing Phase 4 sweeper unchanged; `at_door` bookings have NULL `reservation_expires_at` and are skipped naturally.

_Tests_
- [x] At-door booking: status=paid + payment_method=at_door immediately + Stage 2 email with `tickets_confirmed` template.
- [x] Beforehand booking: status=booked + payment_method=null + payment_reference set + Stage 1 email; mark-paid → status=paid + payment_method=bank_transfer + Stage 2 email; second mark-paid is idempotent.
- [ ] ~~PayPal webhook replay / signature failure / double-capture prevention~~ — deferred to Phase 6b.

**Exit criteria.** ~~End-to-end PayPal payment converts BOOKED → PAID and emails the QR.~~ Bank-transfer event marked paid by an event_admin → status flips, Stage 2 email with inline QR sent. At-door event creates registration that's immediately valid (status=paid, QR in the first email, no payment surface). Donation event same as at-door. Confirmation emails are styled with the event color theme; inline QR present the moment a ticket is paid.

**Notes.**

- 2026-05-06 — Phase 6 closed for the **MVP-shippable** scope (bank transfer, at-door, donation). PayPal + refund are tracked in **Phase 6b** below; they aren't blockers for the first events.
- Smoke verified end-to-end against the seeded Dao Dance event (IBAN `DE89 3704 0044 0532 0130 00` injected by the seed): beforehand booking → Stage 1 receipt → admin mark-paid → Stage 2 with inline QR → second mark-paid idempotent → switching event to `at_door` → next booking is paid immediately + Stage 2 fires at once. `email_log` shows three rows with `status=sent`.
- Emails go through SES via the new `Raw` content path when attachments are present. Inline QRs land as `image/png` parts with `Content-ID: <qr-N@tickets>`; the HTML body references them via `cid:qr-N@tickets`. SES Configuration Set unchanged; reused the psychedelic-rock.com sender from `tickets-psyrock-com`.
- Marker syntax change: `<<cid:xxx>>` instead of `{{cid:xxx}}` because the latter collides with `text/template`. Documented at the call sites.
- New backend packages/files: `internal/mail/render.go` (themed HTML + raw MIME assembly), `internal/booking/admin.go` (HandleListBookings + HandleMarkPaid).
- Per-event bank fields are nullable; events that don't take bank transfers leave them empty and the Stage 1 email simply omits the IBAN block (currently rare since Phase 6 only ships bank-transfer for `beforehand` events; with PayPal in 6b the editor will surface payment-method selection cleanly).
- The Phase 4 banner showing "PayPal + bank transfer" is currently rendered as bank-only — PayPal listing will be added when 6b lands.

**Deliverability fixes** (applied 2026-05-06 after first Stage 2 emails landed in Gmail spam):

- [x] Switched all SES sends to `Raw` content (even when no attachments) so we control the headers explicitly. Previously the no-attachment path used `Simple` and Phase-6 added `Raw` only when attachments were present, leading to inconsistent header behavior.
- [x] **3-deep nested MIME** (`multipart/mixed > multipart/alternative > [text/plain, multipart/related > text/html + image/png]`) matching the canonical Gmail/Apple Mail/Outlook shape. Previously the QR was a sibling of the alternative, which works but is less universally trusted than the nested form.
- [x] **`List-Unsubscribe`** + **`List-Unsubscribe-Post: List-Unsubscribe=One-Click`** headers (RFC 8058). Configured via `MAIL_UNSUBSCRIBE_ADDRESS` env var. Required for Gmail bulk-sender compliance and improves placement on transactional too.
- [x] **`From` with display name** (e.g. `tickets-general <noreply@psychedelic-rock.com>`) via `MAIL_FROM_DISPLAY_NAME`. Helps recipient recognition; major filters score it higher than bare addresses.
- [x] **Explicit `Date` and `Message-ID`** headers. `Message-ID` uses the From-domain for DKIM/DMARC alignment.
- [x] **Random boundaries** (was static `tg-related-boundary` etc.). No collision risk; less predictable to filters.
- [x] **`quoted-printable` for text/plain and text/html** (was raw `8bit`). UTF-8 content with German umlauts now travels through SMTP cleanly; `8bit` is allowed under SMTP 8BITMIME but conservatively-tuned filters penalize it.
- [x] Mirrored the working pattern from `tickets-psyrock-com/src/lib/ses.ts` (the sibling project's SES sender) which has the same headers and delivers reliably.

**Outstanding deliverability tasks** (Phase 11 / launch-readiness territory; non-blocking for first events that send to a verified address):

- [ ] When tickets-general gets its own SES identity (separate from the psyrock-com creds), redo SPF/DKIM/DMARC for that domain. The base project already has them on `psychedelic-rock.com`, so reused creds inherit good reputation; a new domain starts cold.
- [ ] Honor `List-Unsubscribe` clicks: today the header is informational. A real `unsubscribe@…` mailbox with a webhook handler will land in Phase 9 (newsletter), since transactional emails can claim "unsubscribe-by-emailing" without an HTTP endpoint.
- [ ] Phase 9's bulk newsletter MUST add: `Precedence: bulk` header (transactional emails should NOT have this), per-recipient unsubscribe tokens in the URL, and a clear suppression list.
- [ ] Optional: BIMI record for branded indicator next to the From line in Gmail. Requires VMC certificate; nice-to-have, not blocking.

---

## Phase 6b — PayPal.me handle per event + test mode

**Goal.** Let event admins offer PayPal as a faster alternative to bank transfer **without** the platform onboarding a merchant account. Admin pastes a PayPal.me handle per event; the booking form renders a deep-link to `paypal.me/<handle>/<amount>EUR`; the booker pays out-of-band; the admin still uses `mark-paid` to confirm — same flow as bank transfer, just a more convenient checkout for the booker.

Plus a per-event **test mode** toggle: bookings on a test-mode event auto-flip to `paid` immediately (no real PayPal/bank involvement), so admins can rehearse the full pipeline (email rendering, inline QR, ticket page) before going live with a real event.

**Prereqs.** Phase 6.

**Why not full PayPal Orders API auto-capture?** Per-event payee routing requires a PayPal Marketplace / Partner Connect agreement (multi-week onboarding, partnership status). One central platform PayPal Business account is the only realistic path to auto-capture, but it means money lands in the platform account and must be transferred to event organizers manually — not what the user wants. The PayPal.me approach matches the actual mental model ("pay the organizer directly") and reuses the existing manual-confirmation flow. Full API integration is parked in **Open follow-ups** at the bottom of this section, available if/when volume justifies the engineering + compliance cost.

**Todos.**

_Schema_
- [x] Migration `00006_paypal_test_mode.sql`: `events.paypal_handle TEXT` (nullable), `events.payment_test_mode BOOLEAN NOT NULL DEFAULT false`. Drop+recreate `bookings.payment_method` CHECK to include `'test'`.

_Event editor (admin frontend)_
- [x] PayPal block under the existing bank-transfer block (only shown when `payment_timing = 'beforehand'`): single text input "PayPal.me-Benutzername" with explanatory hint showing the resulting `paypal.me/<benutzername>/<betrag>EUR` URL.
- [x] **Test-Mode** toggle (independent of payment timing) — amber-bordered card with explicit "Vor dem Live-Gang abschalten und Test-Buchungen löschen." warning.

_Public event detail / API_
- [x] `paypal_handle` and `payment_test_mode` returned in `GET /api/events/{slug}` (and `/api/admin/events/{id}` for the admin form prefill).

_Booking flow_
- [x] `event.payment_test_mode = true` → status=paid + payment_method='test' + Stage 2 email fires immediately, subject prefixed `[TEST]`. Body opens with "[TEST-MODUS]" copy so a real recipient who somehow sees it can't be misled.
- [x] beforehand + test_mode=false → unchanged from Phase 6 (status=booked, Stage 1 email).
- [x] Stage 1 email gains "Oder per PayPal: paypal.me/<handle>/<amount>EUR" line in both DE and EN sections when `paypal_handle` is set.

_Public booking form (frontend)_
- [x] Beforehand banner now shows the PayPal.me deep-link button (PayPal blue `#003087`) below the IBAN block when handle set; when only PayPal is set, IBAN block disappears. When neither is set, banner shows just "Zahlung im Voraus" generic copy.
- [x] **Test-mode banner** above the form: amber, bilingual ("⚠ TEST-MODUS — Buchungen werden sofort bestätigt, ohne dass eine echte Zahlung ausgelöst wird." / "⚠ TEST MODE — Bookings are confirmed instantly without real payment.").
- [x] Submit button copy flips to "Test-Buchung absenden" in test mode.
- [x] `/events/{slug}/booked` confirmation page: when redirect carries `&test=1`, shows amber banner + "[TEST] Buchung simuliert" headline + explanation.

_Admin dashboard_
- [x] Bookings list rows for `payment_method='test'` get a small `TEST` pill next to the contact name (amber).
- [x] "Test-Buchungen löschen" button surfaces in the section header when at least one test row exists; calls `POST /api/admin/events/{id}/bookings/purge-test`.
- [x] Each non-canceled row also has a "Stornieren" button (red); calls `POST /api/admin/bookings/{id}/refund` with confirm dialog.

_Refund_
- [x] `POST /api/admin/bookings/{id}/refund` (RBAC: `ActionBookingRefund`). Cancels booking + cascades tickets to canceled. Sends `booking.refund_notice` email. Idempotent: already-canceled returns `{already_canceled: true}` without re-sending.
- [x] No automatic PayPal/bank refund (we never had the API integration). Refund email tells the booker the organizer will send the money back via the original channel.

**Exit criteria.** Admin can paste a PayPal.me handle on an event and bookers see a working "Mit PayPal bezahlen" link in the Stage 1 email + booking form. Admin can flip an event into test mode, confirm the entire pipeline (booking → instant paid → Stage 2 email with QR → ticket page), and flip it back without code changes.

**Notes.**

- 2026-05-06 — Phase 6b complete. Smoke verified end-to-end against the seeded Dao Dance event (paypal_handle="daodance"):
  1. Beforehand booking → Stage 1 email (`booking.registration_receipt`) sent (body includes "Oder per PayPal: https://paypal.me/daodance/65EUR" alongside IBAN).
  2. Flip event `payment_test_mode=true`, book again → status=paid, payment_method=test, Stage 2 email subject `[TEST] Tickets bestätigt: …`.
  3. Refund the beforehand booking → status=canceled, tickets canceled, `booking.refund_notice` email sent.
  4. Purge-test → returns `{deleted: 1}`, dashboard now shows zero test rows.
- The PayPal.me deep-link uses the live quote total when available; if the booker hasn't selected anything yet, the banner shows "PayPal-Link erscheint hier, sobald der Betrag berechnet ist." (no broken zero-amount link).
- Test-mode visual treatment: amber border + amber background on the form + amber pill in the dashboard. Color is intentionally not the event's primary color so test bookings stand out from the event's own branding.
- Purge endpoint deletes via `DELETE FROM bookings WHERE event_id=$1 AND payment_method='test'` — the existing CASCADE on participants/tickets/coupon_redemptions handles the rest.
- Refund endpoint never moves money — the platform never had it. Email copy makes this clear: "Falls du bereits gezahlt hast, erhältst du den Betrag in Kürze zurück — der Veranstalter meldet sich."

_Open follow-ups (parked, not scheduled)_:

_Open follow-ups (parked, not scheduled)_:
- [ ] Full PayPal Orders v2 API integration (auto-capture, webhook, refund) using a central platform PayPal Business account. Adds ~1 week of work plus PayPal compliance review. Decide if/when based on operational pain — manual mark-paid is fine until it isn't.
- [ ] Stripe as an alternative to PayPal — possibly cleaner per-event payouts via Stripe Connect / Express accounts. Same engineering cost as full PayPal, similar tradeoffs. Not on the table until the manual flow proves insufficient.

---

## Phase 9 — Public landing & per-event theming polish [done first pass]

**Goal.** The public side feels intentional and minimal, like the base project, with per-event color flowing into both web and email.

**Prereqs.** Phase 4.

**Context to load.** Frontend public routes, email templates, event color set.

**Todos.**
- [x] Front page (`/[locale]/events`): chronological list of future public events. Card-style with banner-as-hero, themed gradient overlay, date/location overlay, description preview underneath. Thin top header with site title + minimal nav.
- [x] Per-event landing (`/[locale]/events/[slug]`): full-width hero with banner image (50% opacity over event color_primary, gradient fade to bottom), centered title in light tracked-out type, date pill, location, "Tickets ansehen ↓" CTA. Below: 2-col meta strip (Datum/Beginn/Ort/Dauer/Plätze) with uppercase tracked labels. Description, program card themed in primary, booking section in a white card. Footer with event name + tickets-general.
- [x] Email templates use the same color set as the event's web page (one source of truth — landed in Phase 6's `RenderBilingualThemed`).
- [x] Default theme matches base project's EP release party palette (`#5E576A` / `#F5F1EE` / `#1A1A1A` from Phase 2 defaults).
- [x] **Banner display fix**: `proxy.ts` (next-intl middleware) was 307-redirecting `/banners/*` requests to `/de/banners/*`, breaking the rewrite to backend. Added `banners` to the matcher exclusion list. Banners now stream cleanly through the frontend dev server.
- [x] **Events list on root `/[locale]`** — replaced the placeholder home page with the events listing so visitors hit something useful immediately. Both `/` and `/events` render the same shared `<EventsList />` component (root is canonical, `/events` kept as alias). Public layout extracted into `<PublicShell />` (header + main + footer with legal links).
- [x] **Static legal pages** — `/[locale]/impressum` (§ 5 TMG, § 18 MStV) and `/[locale]/datenschutz` (Art. 13 DSGVO). Both rendered as server components inside `PublicShell`. Operator-fillable fields are visible amber-bordered "[BITTE AUSFÜLLEN]" markers on the page itself (not hidden HTML comments) so they cannot be missed during pre-launch review. Footer links from every public page reach both in 1 click via the shared `<LegalLinks />` component (also surfaced inside the themed event-detail and booked-confirmation footers).
- [x] **Single-day event meta strip** — drop the "Dauer bis …" cell when `starts_at` and `ends_at` fall on the same calendar day. Layout switched to a `cells[]` array with index-based right/left alignment so adding/removing cells doesn't break the centerline grid.
- [x] **OpenStreetMap location preview** — new client-side `<EventLocationMap />` geocodes the event's `location` string via Nominatim once, caches the result in `localStorage` (`tg.geocode.v1`), and embeds the OSM `/export/embed.html` iframe centered on the point with a marker. Loading skeleton + "Auf OpenStreetMap suchen ↗" fallback when geocode fails. Mounted between meta strip and description on the event-detail page (only when location is set). Datenschutz section 9 added to disclose the third-party fetch (IP + browser info → OSM Foundation servers, Art. 6 (1) f DSGVO basis).
- [ ] Mobile responsiveness audit (deferred — current layout uses sm: breakpoints liberally; visual review on real devices outstanding).
- [ ] Lighthouse pass: performance > 90, a11y > 95 (deferred — no audit run yet).
- [ ] Print-friendly ticket page.

**Exit criteria.** Eyeball test: public site looks like the base project's vibe but is themable per event; emails match.

**Notes.**

- 2026-05-07 — Phase 9 first pass (public landing) complete; pulled forward from its position in the index because the user wanted a demoable ticket page today. Mobile audit, Lighthouse pass, and print-ticket-page polish remain deferred (non-blocking for the demo).
- **Banner bug** (the one that triggered this push): `/banners/<slug>` was returning 307 from the next-intl middleware, which redirected to `/de/banners/<slug>` — a path the next.config rewrite doesn't proxy. Fix in `frontend/src/proxy.ts`: matcher now excludes `banners` alongside `api`, `_next`, `_vercel`, and dotted paths. Direct backend hits + frontend-proxied hits now both return the PNG with `Content-Type: image/png`.
- **Color-set propagation**: the event's `color_primary` / `color_secondary` / `color_text` flow through both pages as inline styles + CSS variables on the page wrapper. The booking-form card inverts to a white background with neutral text so the form remains readable regardless of the event's hero colors.
- New listing card design: aspect-ratio 16:9 (mobile) / 2:1 (desktop), banner with gradient overlay, hover scale on image, color-tinted footer strip with description.
- Landing-page hero gradient: transparent at top → `${color_primary}AA` at 70% → solid `color_primary` at the bottom. The CTA button is a transparent pill with the text-color border, sits comfortably on the hero regardless of the underlying banner.
- Pulled forward at the user's request to demo the public ticket page today; phase numbering kept as-is (Phase 9 stays Phase 9 in the section headers; index updated earlier to clarify execution order).
- 2026-05-07 — root page now lists events directly (no more empty landing). `/[locale]/events` kept as alias for any bookmarks/links that already exist. Tests: `/de`, `/de/events`, `/de/impressum`, `/de/datenschutz`, `/en`, `/en/impressum` all return 200 with full SSR markup; impressum HTML contains 16 visible "[BITTE AUSFÜLLEN]" markers (counted in smoke).
- **Why visible TODO markers instead of HTML comments?** § 5 TMG and Art. 13 DSGVO require concrete, accurate data. Pre-launch operator MUST replace the placeholders. Hidden TODO comments would let an unfilled Impressum slip into production — visible amber boxes make that impossible to miss during the "look at every public page once" pre-launch pass.

---

## MVP launch path

The next four phases ship the platform to its first live event, in this fixed order. Each phase unlocks the next: waitlist closes the booking-flow gap for limited events, booking management makes the admin side workable at scale, user management retires the manual-SQL admin onboarding, and roll-out wraps it all into a deployable release. Door scanning and newsletter — historically Phase 7 and Phase 11 — move to **Post-MVP** because the first events can run with a printed name list and no broadcast mail.

---

## Phase 10 — Waitlist & auto-promotion [done · admin UI follow-up still open]

**Goal.** When capacity is full on a limited event, push to waitlist; on cancellation, promote per event policy. Entire subsystem is **gated on `events.participant_limit IS NOT NULL`** — for unlimited events the waitlist code path is never entered.

**Prereqs.** Phase 4, Phase 6.

**Context to load.** `waitlist_entries` schema; existing capacity check at `backend/internal/booking/booking.go:102`; cancel hook from Phase 5; mark-paid/refund flows from Phase 6.

**Locked design decisions** (2026-05-08):

1. **Public booking UX = implicit.** Submitting the booking form on a full event silently lands the user on the waitlist; they're redirected to a "Du bist auf der Warteliste" page that explains they'll be notified when a spot opens. No two-step opt-in.
2. **Pre-emptive UI on full events.** When a visitor opens an event whose limit is reached, the public event detail page shows a banner above the booking form — "Veranstaltung ist ausgebucht — Eintrag in die Warteliste ist möglich." Form fields and behavior are unchanged; the banner just sets expectations so the waitlist outcome isn't a surprise.
3. **Paid-event promotion = notify-all, first-to-claim wins.** When seats open: send "spot opened" email to every waiter whose `requested_seats ≤ open_seats`. The first to click their claim link locks the spot atomically (event-row `FOR UPDATE` + remaining-seats decrement) and gets a tentative booking with a 24h reservation expiry. Subsequent claimers see "Der Platz wurde bereits vergeben." Reservation expiry → re-promote.
4. **Donation-event promotion = auto-promote.** No claim window, no race. The cancel hook walks the waitlist FIFO and assigns the oldest fitting waiter directly to a paid (donation) booking. Emails the user "Du hast einen Platz!".
5. **Multi-seat allocation = FIFO with skip-if-doesn't-fit.** When N seats open, scan the waitlist by `created_at ASC`. For each waiter: if `requested_seats ≤ remaining`, promote and decrement; else skip and keep them waiting. Stop when remaining=0. Unit-tested for full fit, partial skip, all skip, empty queue, and the multi-skip case (waiter-A skipped, waiter-B promoted, waiter-A still waiting).
6. **Coupon validity at claim = locked at join.** If a coupon was applied when joining the waitlist, it stays valid through claim. No re-validation. Coupon redemption is recorded at booking creation (i.e. at claim for paid events, at auto-promote for donation events), not at waitlist-join — so the redemption count is only consumed when the seat is actually taken.
7. **Self-service removal = on.** Every waitlist email ("joined", "spot opened", "still waiting") includes an HMAC-signed `Auf Warteliste entfernen` link with purpose-domain `waitlist_remove`. One click → status='removed', no admin needed.
8. **Position visible to waiter.** The "joined" email includes "Du bist Position N von M" since most users find it reassuring.

**Todos.**

_Schema_
- [ ] Migration `00007_waitlist_status.sql`:
  - `events.waitlist_claim_window_hours INTEGER NOT NULL DEFAULT 24` — admin-tunable per event.
  - `waitlist_entries.status TEXT NOT NULL DEFAULT 'waiting'` with CHECK constraint `('waiting', 'promoted', 'fulfilled', 'expired', 'removed')`. Cleaner than juggling NULLs across `promoted_at` / `fulfilled_booking_id`.
  - `waitlist_entries.coupon_id UUID NULL` — captures the coupon at join so claim doesn't have to reconcile with selection_json.
  - Index `waitlist_entries (event_id, status, created_at)` — drives the FIFO promotion query.

_sqlc queries_ (`backend/sqlc/queries/waitlist.sql`)
- [ ] `CreateWaitlistEntry`, `ListWaitingForEventLocking` (FIFO with `FOR UPDATE`), `ListWaitlistForEvent` (admin view), `GetWaitlistEntryByID`, `MarkWaitlistPromoted(id, deadline)`, `MarkWaitlistFulfilled(id, booking_id)`, `MarkWaitlistExpired(id)`, `MarkWaitlistRemoved(id)`, `LockEventForCapacity(id)` (event-row lock).

_Booking flow change_
- [ ] At `booking.go:102`: replace the `errCapacityReached` early return with a branch — if `event.HasParticipantLimit()` and over capacity, create a `waitlist_entry` instead, send `waitlist.joined` email, return `{status: "waitlisted", waitlist_id, position}`. The `/quote` endpoint gains a `capacity_full bool` flag for the frontend banner.
- [ ] Capacity utilization (existing `CountActiveSeatsForEventLocking`) implicitly already counts BOOKED + PAID. Tentative-from-waitlist bookings carry status='booked' so they're already counted. No query change.

_Promotion service_ (`backend/internal/waitlist/`)
- [ ] `Service.PromoteAfterCancel(ctx, eventID)` — single entry point called by the cancel/refund/reservation-expiry hooks. Inside one tx: lock event, compute `remaining = participant_limit - active_seats`, walk waiters FIFO with skip-if-doesn't-fit:
  - **Donation event**: directly create paid booking + tickets per fitting waiter, mark waitlist `fulfilled`, send `waitlist.promoted_donation` email.
  - **Paid event**: send `waitlist.spot_opened` to all waiters whose `requested_seats ≤ remaining` (no waitlist-row state change yet — claim is what transitions).
- [ ] `Service.HandleClaim(ctx, claim_token)` — verifies HMAC token (purpose `waitlist_claim`), locks event, re-checks `remaining`, atomically transitions waitlist row to `promoted` + creates tentative booking with `reservation_expires_at = now() + waitlist_claim_window_hours`, returns booking redirect URL. Lost claims (capacity exhausted before they clicked) → 409 with `error: claim_unavailable`.
- [ ] `Service.HandleRemove(ctx, remove_token)` — verifies HMAC (purpose `waitlist_remove`), idempotently sets status='removed'. Always returns 200 to user-facing route.
- [ ] Cancel/refund hook: existing `HandleRefund` adds a final `waitlist.PromoteAfterCancel(ctx, eventID)` call (in a goroutine so the refund response isn't blocked).
- [ ] Reservation expiry sweeper: confirm Phase 5 has one — if yes, hook `PromoteAfterCancel` into it; if no, add a 1-minute ticker that cancels expired tentative bookings then triggers promotion.

_Email templates_ (`backend/internal/booking/email.go`-style)
- [ ] `waitlist.joined` (DE+EN): "Du bist auf der Warteliste für …. Position N von M. Wir melden uns sofort, sobald ein Platz frei wird." Includes self-removal link.
- [ ] `waitlist.spot_opened` (DE+EN, paid only): "Ein Platz ist frei geworden! Sichere ihn dir hier: [Claim-Link]. Du hast 24 Stunden Zeit zu zahlen, sonst geht der Platz an die nächste Person." Includes open-seat count and self-removal link.
- [ ] `waitlist.promoted_donation` (DE+EN, donation only): same shape as Stage 2 confirmation; user goes straight to a paid booking with QR.
- [ ] `waitlist.removed_self` (DE+EN): courtesy confirmation after self-removal.

_Public frontend_
- [ ] BookingForm shows pre-emptive "ausgebucht — Warteliste verfügbar" banner when quote response has `capacity_full=true`. Submit button copy changes to "Auf die Warteliste eintragen" but the form is otherwise unchanged.
- [ ] New `/events/{slug}/waitlisted` confirmation page (variant of `/booked`): explains the waiter's position, when they'll be contacted, and includes a "Eintrag entfernen" link.
- [ ] New `/events/{slug}/claim/{token}` page: calls backend claim handler. Success → redirect to `/booked` with the tentative booking's reference. Failure → "Der Platz ist leider schon vergeben — du bleibst auf der Warteliste."
- [ ] New `/events/{slug}/waitlist-removed/{token}` page: one-click self-removal endpoint hit; renders confirmation.

_Admin UI_ (`/admin/events/{id}/waitlist`)
- [ ] Hidden / disabled when `event.participant_limit IS NULL`.
- [ ] Table: name, email, requested seats, status pill, joined, promoted_at (if any), claim_deadline countdown (if active).
- [ ] Per-row actions: "Manuell promoten" (skips queue, behaves like a one-target spot_opened), "Entfernen" (sets status='removed', sends `waitlist.removed_admin`).
- [ ] Counter strip: waiting / promoted / fulfilled / expired / removed.

_Tests_
- [ ] **Allocation** (pure function): full fit, partial skip, all skip, empty queue, multi-skip-then-promote (waiter A wants 3, waiter B wants 1, 1 seat free → A skipped, B promoted, A stays waiting).
- [ ] **Concurrent claim**: 2 goroutines hit `HandleClaim` for the same row → exactly one wins, the other gets `claim_unavailable`.
- [ ] **Concurrent claims for different rows when 1 seat remains**: 2 different waiters race claims → one wins, one gets `claim_unavailable` after the lock-and-recount.
- [ ] **FIFO ordering**: 5 waiters in known order, capacity opens by 1, oldest is the first/only to be addressable.
- [ ] **Re-promotion on expiry**: claim row expires → reservation expiry sweep cancels the tentative booking → `PromoteAfterCancel` re-runs → next waiter gets a `spot_opened` email.
- [ ] **Donation auto-promote**: cancel on a donation event → next fitting waiter gets a paid booking + email; no claim email.
- [ ] **Coupon stays valid**: waitlist with applied coupon, then coupon expires/maxes → claim still proceeds at the original discounted price (redemption recorded at claim, not join).
- [ ] **Self-removal**: signed-token endpoint is idempotent; second call returns the same 200; foreign tokens fail.
- [ ] **Unlimited events**: 100 concurrent over-capacity bookings on `participant_limit IS NULL` create exactly 0 waitlist rows.

**Exit criteria.** For both limited paid and limited donation events, an over-capacity booking lands on the waitlist (no opt-in step); a subsequent cancellation triggers the right promotion behavior automatically. Unlimited events never produce waitlist rows. The allocation algorithm has full unit-test coverage of the multi-seat skip/fit cases.

**Notes.**

- 2026-05-08 — Design decisions locked from user Q&A: implicit waitlist, pre-emptive banner, notify-all-claim for paid, auto-promote for donation, FIFO with skip-if-doesn't-fit, coupon-stays-valid, self-removal, position visible.
- 2026-05-09..10 — Phase 10 shipped live on kreise.berlin: schema 00007, sqlc waitlist queries, pure FIFO allocator with 11 unit tests, booking-flow branch on over-capacity, `PromoteAfterCancel` orchestrator (donation auto-fulfill + paid spot-opened broadcast), `HandleClaim` + `HandleRemove`, cancel/refund hooks, per-ticket-cancel hook, reservation-expiry sweeper hook with `expirePromotedWaitlist`, four email templates (`waitlist.joined`/`spot_opened`/`promoted_donation`/`removed_self`), admin one-shot `/admin/events/{id}/waitlist/promote` rotate-now endpoint, and the four public frontend pages (BookingForm capacity-full banner + `/waitlisted`, `/waitlist/claim/{token}`, `/waitlist/remove/{token}`). End-to-end smoke verified against tib26-1 (donation, limit=3). **Still open:** dedicated `/admin/events/{id}/waitlist` admin UI; DB-backed concurrency tests (allocator unit tests cover the algorithm, the race-against-event-lock is exercised in production but not in CI).

---

## Phase 8 — Booking management (admin ops at scale) [shipped 2026-05-11]

**Shipped:** migration `00009_booking_search_indexes.sql` (pg_trgm + two trigram indexes on `lower(contact_email|name)` + composite `(event_id, status, created_at desc)`). New sqlc queries `SearchBookingsForEvent`, `CountSearchBookingsForEvent`, `ListExpandedBookingsForEvent`, `UpdateBookingContactEmail` (all using `sqlc.narg()` for nullable filter params). New handlers: paginated list with summary scoped to filter; `POST /api/admin/bookings/{id}/resend-confirmation` (Stage 1 for booked / Stage 2 for paid); `PATCH /api/admin/bookings/{id}` (contact_email only, audit-logged old/new); `GET /api/admin/events/{id}/bookings/export.csv` (streams with `string_agg` participant names); `POST /api/admin/bookings/bulk-mark-paid` (partial-failure shape `{succeeded, failed}`). Dedicated `/admin/events/{id}/bookings` page with URL-synced filter bar (q, status, payment_method, sort), 50-per-page pagination, ←/→ keyboard shortcuts, summary strip, kebab actions per row (mark-paid / resend / edit email / refund), bulk select + bulk mark-paid, CSV export button. Inline list on the event detail page replaced with a thin "Buchungen verwalten →" CTA + counter strip.

**Pending (deferred polish):** tests for pagination boundaries, search across each column, CSV escaping, bulk partial-failure. Rate limit on resend-confirmation (3/day per booking) not yet enforced; the audit log makes abuse traceable in the meantime.



**Goal.** Admins can find, filter, sort, and act on bookings at scale (hundreds to thousands per event). The current inline list on the event admin page works for ten bookings — it falls over at a hundred. This phase moves the bookings UI to a dedicated page with server-side search/filter/sort/pagination, adds a couple of missing per-booking actions (resend confirmation, edit contact email), supports bulk mark-paid for batch bank-transfer reconciliation, and exports a CSV for accounting handoff.

**Prereqs.** Phase 5 (tickets), Phase 6 (payments), Phase 6b (test mode + refund).

**Context to load.** `frontend/src/app/[locale]/admin/events/[id]/page.tsx` (current inline list), `backend/internal/booking/admin.go`, `backend/sqlc/queries/bookings.sql`.

**Todos.**

_Backend_
- [ ] Replace `ListBookingsForEvent` with a paginated, filterable query: `GET /api/admin/events/{id}/bookings?status=&payment_method=&q=&from=&to=&limit=50&offset=0&sort=created_at&order=desc`.
- [ ] `q` searches across contact_email, contact_name, and reference (ILIKE, case-insensitive). Test that an admin can locate "schmidt" and "SCHMIDT@" both hit.
- [ ] Response shape: `{bookings: [...], total, limit, offset, summary: {total_count, paid_count, paid_revenue_minor, booked_count, canceled_count}}`. Counts and revenue sum scoped to the **current filter** so the dashboard summary reflects what's on screen.
- [ ] `POST /api/admin/bookings/{id}/resend-confirmation` — re-sends Stage 1 if status=booked, Stage 2 if status=paid. Audit-logged. Rate-limited per booking (3/day) to prevent abuse.
- [ ] `PATCH /api/admin/bookings/{id}` — narrow surface, **only `contact_email`** in MVP (typo fixes). Audit-logged with old/new value.
- [ ] `GET /api/admin/events/{id}/bookings/export.csv` — streams a CSV of the current filter (admin downloads, no pagination). Columns: reference, status, contact_name, contact_email, total_eur, currency, payment_method, paid_at_iso, created_at_iso, participant_count, participant_names_pipe_separated.
- [ ] `POST /api/admin/bookings/bulk-mark-paid` — accepts `{booking_ids: [...], event_id}`, validates all rows belong to that event + admin has rights, marks each booking-and-tickets paid, fires Stage 2 emails per row. Per-row failures don't poison the batch — response shape is `{succeeded: [...], failed: [{id, reason}]}`.
- [ ] Indexes: `bookings (event_id, status, created_at DESC)` plus a trigram or expression index on `lower(contact_email)` and `lower(contact_name)` so search stays sub-100ms past 10k rows.
- [ ] Tests: pagination boundaries (offset > total), search across each column, summary counts honor filter, CSV escaping (commas, quotes, newlines in names), bulk mark-paid partial-failure handling, RBAC scoping (event_admin/event_manager only on assigned events).

_Admin frontend_
- [ ] Move bookings UI out of `/admin/events/[id]` into a dedicated `/admin/events/[id]/bookings` page. The event detail page surfaces a "Buchungen verwalten →" CTA showing the live count and a tiny breakdown (paid / booked / canceled).
- [ ] Filter bar: status select, payment_method select, date range, free-text search, sort dropdown. URL-synced (`?q=…&status=paid&page=2`) so admins can share / refresh / back-button / bookmark a filter.
- [ ] Pagination: 50 per page; pager UI plus "X bis Y von Z" indicator. Keyboard ←/→ shortcuts to flip pages.
- [ ] Summary strip at the top: total count, paid revenue, status breakdown — reflects the current filter. "Filter zurücksetzen" link clears and shows the un-filtered totals.
- [ ] Per-row actions condensed into a kebab menu: mark paid, refund, resend confirmation, edit contact email. Mark paid + refund stay confirmable; the rest are 1-click.
- [ ] Bulk select with header checkbox + "Alle auf Seite". Bulk action: mark paid (for batch bank-transfer reconciliation). Refund deliberately NOT bulk-able (too easy to misclick).
- [ ] "CSV exportieren" button in the page header — exports the current filter. Shows toast "X Buchungen exportiert".
- [ ] Empty states per filter: "Keine Buchungen mit diesen Filtern" (with reset link).
- [ ] Test-mode pill + purge-test button carried over from the current dashboard.

**Exit criteria.** An event admin can find any booking by name / email / reference within 5 seconds among 1000+ rows, perform every booking-level action from the same page, batch-mark-paid 30 bank-transfer rows in one click, and export a filtered CSV ready for accounting. The legacy inline list on the event detail page is gone and replaced by a CTA to the dedicated page.

**Notes.**

---

## Phase 13 — User management (admin lifecycle) [MVP]

**Goal.** Onboard admins without manual SQL. Global admins create event admins; event admins assign managers to events they own. Self-service password change + reset. Force-logout when revoking access. Audit log captures every change.

**Prereqs.** Phase 1 (auth + RBAC), Phase 2 (event scoping).

**Context to load.** `users` table, `event_admins` / `event_managers` join tables, `auth/`, `authz/`, audit log.

**Todos.**

_Schema_
- [ ] `users.password_version INT NOT NULL DEFAULT 1`. Sessions store the version they were minted at; on validation, mismatch → reject. Force-logout (or password change) increments the column.
- [ ] `setup_tokens` table: `id (UUID PK), user_id (FK), token_hash (TEXT UNIQUE), expires_at (TIMESTAMPTZ), used_at (TIMESTAMPTZ NULL)`. Single-use, hashed at rest.
- [ ] `password_reset_tokens` table: same shape, separate purpose so a leaked setup token can't reset an existing password.

_Backend_
- [ ] `POST /api/admin/users` (global_admin only): create user with role + email. Generates a setup token, sends `admin.invite` email with `/setup/<token>` link. User row created with `active=false`, `password_hash=NULL`.
- [ ] `POST /api/admin/users/{id}/deactivate` and `/reactivate` (global_admin only). Deactivate increments `password_version` (force-logout).
- [ ] `POST /api/admin/users/{id}/role` — change role (global_admin only). No-op if already that role.
- [ ] `GET /api/admin/users` — list with last_login_at, active, role, session_count.
- [ ] `POST /setup/{token}` — body: `{password, password_confirm}`. Validates token (exists, not used, not expired), hashes with argon2id, sets `users.active=true`, marks token used, mints first session.
- [ ] `POST /api/admin/account/password` — self-service: requires current password, sets new one. Increments password_version (drops other sessions). 12-char minimum.
- [ ] `POST /forgot-password` — public, IP+email rate-limited (3/day per email), generates a reset token, sends `admin.password_reset` email. Always returns 200 to avoid leaking which emails exist.
- [ ] `POST /reset-password/{token}` — same shape as setup; consumes token, sets new hash.
- [ ] `POST /api/admin/events/{id}/managers` and `DELETE /api/admin/events/{id}/managers/{user_id}` (event_admin scoped to their events): assign / unassign existing event_managers. Cannot assign event_admins. Cannot operate on events not owned.
- [ ] Audit: every create / role-change / activate / deactivate / password-change / force-logout / manager-assign action logged with actor, target, payload (no secrets in payload).

_Admin frontend_
- [ ] `/admin/users` (global_admin only): list, "Neuen Admin einladen" form (email + role), per-row deactivate / reactivate / change-role / force-logout. Last-login + session-count visible.
- [ ] `/admin/account` for any logged-in admin: change own password, see own sessions, "Auf allen Geräten abmelden" button (= force-logout on self).
- [ ] `/admin/events/{id}/team` (event_admin scoped): list assigned managers, search-add by email (must already be a registered event_manager — no nested invites in MVP), unassign with confirm.
- [ ] `/setup/{token}` and `/reset-password/{token}` public routes — server components, password form, success → login.
- [ ] `/forgot-password` route with email input, neutral confirmation copy ("Falls ein Account mit dieser Adresse existiert, ist eine E-Mail unterwegs.").

_Email_
- [ ] `admin.invite` (DE+EN): "Du wurdest als <Rolle> eingeladen. Hier ist dein Setup-Link …".
- [ ] `admin.password_reset` (DE+EN): "Setze dein Passwort neu …".
- [ ] `admin.deactivated` (DE+EN, optional courtesy mail when revoked).

_Tests_
- [ ] Setup token: single-use, expiry honored, used token returns generic "ungültig oder abgelaufen".
- [ ] Reset token: same shape; can't reuse a setup token as a reset token (purpose isolation).
- [ ] RBAC: event_admin cannot assign event_admins / cannot touch users on events they don't own / cannot view `/admin/users`.
- [ ] Password change requires current password — no bypass via session-only check.
- [ ] Force-logout actually invalidates extant sessions (existing session validation must drop on `password_version` mismatch).
- [ ] Forgot-password rate limit per IP + per email; always-200 to prevent enumeration.

**Exit criteria.** A global admin can invite a new event_admin who completes setup via email; that event_admin can invite (= assign) a manager to one of their events without SQL or backend access. Deactivation invalidates sessions immediately. All of it audit-logged.

**Notes.**

---

## Phase 12 — Roll-out / launch readiness [MVP]

**Goal.** Production-deployable: security, observability, ops, and the operator checklist that turns a working staging into a live deployment.

**Prereqs.** Phase 10, Phase 8, Phase 13 (everything else in the MVP path).

**Context to load.** `deploy/`, env config, secrets policy, the legal pages from Phase 9.

**Todos.**

_Helm + k8s_
- [ ] Helm chart with: backend Deployment, frontend Deployment, Service, Ingress, ConfigMap, Secret, migration Job (runs pre-upgrade), HPA optional.
- [ ] Postgres: managed (RDS-equivalent) or in-cluster operator — decide and document.
- [ ] **MinIO in-cluster**: Deployment (single replica is fine at MVP), PVC sized for projected banner volume, ClusterIP Service (no Ingress — accessed only by backend), Secret holding root creds. Use `bitnami/minio` chart as a subchart or pin a specific MinIO image.
- [ ] MinIO bucket bootstrap Job: creates the `tickets` bucket on first install, idempotent on upgrade.
- [ ] MinIO backup: nightly CronJob running `mc mirror` to off-cluster destination (S3, R2, or rsync target). Document the restore procedure in the runbook.
- [ ] Backend `STORAGE_*` env vars wired to in-cluster MinIO Service DNS.
- [ ] Banner pass-through endpoint sets `Cache-Control: public, max-age=…` so an Ingress-level or browser cache absorbs repeat reads.
- [ ] Domain + TLS via cert-manager.
- [ ] Probes: liveness on `/healthz`, readiness on `/readyz`.
- [ ] Resource requests/limits per workload (including MinIO).

_Secrets & IAM_
- [ ] Session signing key, PayPal creds, SES creds — sourced from Secret only.
- [ ] SES IAM policy least-privilege (`ses:SendEmail`, `ses:SendRawEmail`, sending domain only).
- [ ] PayPal webhook secret rotation procedure (parked unless Phase 6b's "full PayPal Orders API" is unparked).
- [ ] Rotate SES credentials away from the borrowed `tickets-psyrock-com` set onto a tickets-general-owned IAM user, with a verified sender on the production domain. Update `MAIL_FROM` accordingly.

_Observability_
- [ ] Structured JSON logs to centralized sink.
- [ ] Prometheus metrics: HTTP duration histogram, booking funnel counters, payment success/failure, mail send success/failure.
- [ ] Error tracking (Sentry or equivalent).
- [ ] Audit log retention policy.

_Security_
- [ ] OWASP top-10 self-review (with `/security-review` skill on the final branch).
- [ ] CSP headers, secure/HttpOnly/SameSite cookies, HSTS.
- [ ] Rate limit at ingress + per-endpoint application limits.
- [ ] Dependency vuln scan in CI (`govulncheck`, `npm audit`).
- [ ] Backup + restore drill on Postgres.

_Compliance_
- [ ] **Fill in Impressum** (`/[locale]/impressum`) — replace every visible amber `[BITTE AUSFÜLLEN]` marker with concrete operator data: anbieter / vertretungsberechtigt / contact / optional registry + USt-IdNr. § 5 TMG requires the data be present and directly accessible before the site is publicly reachable.
- [ ] **Fill in Datenschutzerklärung** (`/[locale]/datenschutz`) — replace every amber marker with concrete data (verantwortlicher, contact for data requests, hosting provider, MinIO/S3 provider, AWS DPA link, Stand-Datum). Cross-check that the listed third parties (AWS SES, PayPal, OpenStreetMap) match what's actually wired in production.
- [ ] Per-user data export and deletion endpoints (GDPR Art. 15 / 17).
- [ ] Cookie banner only if any non-essential cookie is set (currently we set none, so confirm before launch).

_Validation_
- [ ] Load test on booking + payment with realistic ticket counts.
- [ ] Production env config doc (every required env var, default, sensitivity).
- [ ] Runbook: how to mark a booking paid manually if PayPal webhook is down; how to roll back a release; how to restore Postgres from backup; how to rotate SES creds.
- [ ] First-event playbook: a checklist the operator runs through before pointing the first real event at this platform (DNS pointed, TLS green, SES out of sandbox, legal pages filled, test-mode flipped off, test bookings purged).

**Exit criteria.** A staging deploy survives the load test and a manual end-to-end run-through of all flows; all security checklist items green; all amber legal placeholders filled in; first-event playbook executed dry-run.

**Notes.**

---

## Post-MVP

These ship after the platform is taking real bookings on real events. Each is independently scoped; ordering between them is open.

---

## Phase 7 — Door scanning / check-in [post-MVP]

**Goal.** event_managers scan QR codes at the door; per-participant check-in.

**Prereqs.** Phase 5.

**Context to load.** `tickets`, QR token verifier, RBAC.

**Todos.**
- [ ] `POST /scan` endpoint (event_manager+): verifies QR token, marks ticket CHECKED_IN, returns participant name + previous check-in time if any.
- [ ] Idempotent: re-scan returns "already checked in at <time>", not an error.
- [ ] Per-participant granularity preserved (each ticket is one participant).
- [ ] Mobile-friendly scanner page using browser camera (no native app).
- [ ] Manual check-in fallback by ticket reference for damaged QR.
- [ ] Audit-log every scan attempt (success and failure).
- [ ] Tests: invalid token, foreign-event ticket, role enforcement, replay, concurrent scans of the same ticket.

**Exit criteria.** A manager opens the scanner page on a phone, scans a real QR, sees green; second scan of the same QR shows "already checked in".

**Notes.**

- Deferred to post-MVP on 2026-05-08: the first events run with a printed name list at the door, then this phase ships shortly after to remove that crutch.

---

## Phase 11 — Newsletter & broadcast mail [post-MVP]

**Goal.** Event-scoped admins can write to current participants and to past-event opt-in participants.

**Prereqs.** Phase 1 (mailer), Phase 4 (participants exist), Phase 2 (admin scoping).

**Context to load.** Mailer, `participants.newsletter_optin`, audit log.

**Todos.**
- [ ] Composer endpoint with subject, body (HTML+text), audience selector.
- [ ] Audience selectors: "current event participants", "past event N participants who opted in", combinable.
- [ ] Test send: post to composer's own email only.
- [ ] Send job: batched, paced for SES rate limits, persisted progress so a crash doesn't double-send.
- [ ] Per-recipient suppression (bounces, complaints from SES feedback).
- [ ] Unsubscribe link with signed token; one-click unsubscribe sets `participants.newsletter_optin=false`.
- [ ] Send log surfaced in admin UI.
- [ ] Frontend composer with theme-aware preview.
- [ ] Tests: audience resolution, opt-in respect, unsubscribe, suppression.
- [ ] GDPR/CAN-SPAM: unsubscribe in every newsletter; postal address footer if shipping commercially.

**Exit criteria.** An event admin can send a themed newsletter to last event's opted-in participants, test-send to themselves first, and recipients can one-click unsubscribe.

**Notes.**

- Deferred to post-MVP on 2026-05-08: no past-event participants exist on day one, so this phase has no real audience to mail until at least one event has run end-to-end.

---

## Conventions for working in this file

- Tick boxes only when the work is on the main branch (or merged to whatever the trunk is). Don't pre-tick aspirational work.
- If a todo turns out to be wrong or unnecessary, strike through (`~~text~~`) with a one-line reason rather than deleting — preserves the audit trail.
- New work that emerges mid-phase: add it to the current phase as a fresh unchecked todo, not a new phase, unless it's genuinely a new vertical.
- If you discover a Phase-N task should have been in Phase N−1, log it in **Notes** and decide: backfill or accept.
- When advancing **Current phase**, write a one-line entry in the previous phase's **Notes** summarizing what landed and any deferrals.
