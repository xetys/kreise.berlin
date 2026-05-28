-- name: CreateWaitlistEntry :one
INSERT INTO waitlist_entries (
    event_id, contact_email, contact_name, locale, requested_seats,
    selection_json, coupon_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetWaitlistEntryByID :one
SELECT * FROM waitlist_entries WHERE id = $1;

-- name: ListWaitingForEventLocking :many
-- FIFO scan of the queue, locked for promotion. Caller MUST run inside the
-- same transaction that took the event-row lock via LockEventForCapacity,
-- so the lock ordering is consistent (event row first, then waitlist rows)
-- and concurrent promote attempts serialize cleanly.
SELECT * FROM waitlist_entries
WHERE event_id = $1 AND status = 'waiting'
ORDER BY created_at ASC
FOR UPDATE;

-- name: ListWaitlistForEvent :many
-- Admin view: every status, every row.
SELECT * FROM waitlist_entries
WHERE event_id = $1
ORDER BY created_at ASC;

-- name: CountWaitingForEvent :one
SELECT count(*)::int AS waiting
FROM waitlist_entries
WHERE event_id = $1 AND status = 'waiting';

-- name: WaitlistStatusCountsForEvent :one
SELECT
    count(*) FILTER (WHERE status = 'waiting')::int   AS waiting_count,
    count(*) FILTER (WHERE status = 'promoted')::int  AS promoted_count,
    count(*) FILTER (WHERE status = 'fulfilled')::int AS fulfilled_count,
    count(*) FILTER (WHERE status = 'expired')::int   AS expired_count,
    count(*) FILTER (WHERE status = 'removed')::int   AS removed_count
FROM waitlist_entries
WHERE event_id = $1;

-- name: PositionInWaitlist :one
-- 1-based position of the given waiter among current waiting rows.
SELECT (count(*)::int + 1) AS position
FROM waitlist_entries w
WHERE w.event_id = $1
  AND w.status = 'waiting'
  AND w.created_at < (SELECT w2.created_at FROM waitlist_entries w2 WHERE w2.id = $2);

-- name: MarkWaitlistPromoted :exec
UPDATE waitlist_entries
SET status = 'promoted', promoted_at = now(), claim_deadline = $2
WHERE id = $1 AND status = 'waiting';

-- name: MarkWaitlistFulfilled :exec
UPDATE waitlist_entries
SET status = 'fulfilled', fulfilled_booking_id = $2
WHERE id = $1 AND status IN ('waiting', 'promoted');

-- name: MarkWaitlistExpired :exec
UPDATE waitlist_entries
SET status = 'expired'
WHERE id = $1 AND status = 'promoted';

-- name: MarkWaitlistRemoved :exec
UPDATE waitlist_entries
SET status = 'removed'
WHERE id = $1 AND status NOT IN ('fulfilled', 'removed');

-- name: LockEventForCapacity :exec
-- Bare row-lock on the event. Pair this with CountActiveSeatsForEvent (no
-- locking variant) inside the same tx so capacity reads are stable.
SELECT id FROM events WHERE id = $1 FOR UPDATE;

-- name: ListExpiredPromotedWaitlist :many
-- Sweeper input: waitlist rows whose claim window has passed. The reservation
-- expiry sweeper cancels the underlying booking; this query lets the waitlist
-- service mark the row 'expired' and trigger re-promotion.
SELECT id, event_id FROM waitlist_entries
WHERE status = 'promoted'
  AND claim_deadline IS NOT NULL
  AND claim_deadline < now();
