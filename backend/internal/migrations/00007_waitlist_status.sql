-- +goose Up

-- Per-event claim-window for paid-event waitlist promotions. When a spot
-- opens we email all fitting waiters; the first one to click their claim
-- link gets a tentative booking with reservation_expires_at = now() +
-- this many hours. Default 24h, admin-tunable per event.
ALTER TABLE events ADD COLUMN waitlist_claim_window_hours INTEGER NOT NULL DEFAULT 24
    CHECK (waitlist_claim_window_hours > 0);

-- Lifecycle status on each waitlist row. Cleaner than juggling NULLs across
-- promoted_at / fulfilled_booking_id when querying.
--   waiting   — sitting in the queue, no spot offered yet
--   promoted  — paid event: claim email sent, claim_deadline running
--   fulfilled — became an actual booking (donation auto-assign, or claim won)
--   expired   — promoted but the claim window passed without action
--   removed   — admin-removed or self-removed via signed link
ALTER TABLE waitlist_entries ADD COLUMN status TEXT NOT NULL DEFAULT 'waiting'
    CHECK (status IN ('waiting', 'promoted', 'fulfilled', 'expired', 'removed'));

-- Capture the coupon at join. Coupon stays valid through claim regardless
-- of expiry / max-uses by then — redemption is recorded at booking creation
-- (claim or auto-promote), not at waitlist-join.
ALTER TABLE waitlist_entries ADD COLUMN coupon_id UUID REFERENCES coupons(id) ON DELETE SET NULL;

-- Replace the bare (event_id, created_at) index with one that drives the
-- FIFO promotion query — most reads filter status='waiting'.
DROP INDEX IF EXISTS waitlist_event_created_idx;
CREATE INDEX waitlist_event_status_created_idx
    ON waitlist_entries (event_id, status, created_at);

-- +goose Down

DROP INDEX IF EXISTS waitlist_event_status_created_idx;
CREATE INDEX waitlist_event_created_idx ON waitlist_entries (event_id, created_at);

ALTER TABLE waitlist_entries DROP COLUMN coupon_id;
ALTER TABLE waitlist_entries DROP COLUMN status;
ALTER TABLE events DROP COLUMN waitlist_claim_window_hours;
