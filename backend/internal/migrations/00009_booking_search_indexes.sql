-- +goose Up

-- pg_trgm gives sub-100ms ILIKE search past 10k rows. Required by the admin
-- bookings search across contact_email + contact_name. CREATE EXTENSION is
-- idempotent; safe across re-runs.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Trigram indexes on lower(email) and lower(name) — the admin search calls
-- ILIKE '%foo%' which can use trigram GIN to skip a sequential scan.
CREATE INDEX IF NOT EXISTS bookings_contact_email_trgm_idx
    ON bookings USING gin (lower(contact_email) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS bookings_contact_name_trgm_idx
    ON bookings USING gin (lower(contact_name) gin_trgm_ops);

-- Composite index for the default filter (status + chronological).
CREATE INDEX IF NOT EXISTS bookings_event_status_created_idx
    ON bookings (event_id, status, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS bookings_event_status_created_idx;
DROP INDEX IF EXISTS bookings_contact_name_trgm_idx;
DROP INDEX IF EXISTS bookings_contact_email_trgm_idx;
-- Leave pg_trgm installed — other tables may eventually use it.
