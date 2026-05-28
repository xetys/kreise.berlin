-- name: CreateBooking :one
INSERT INTO bookings (
    event_id, contact_email, contact_name, contact_phone, locale,
    status, total_amount_minor, currency, coupon_id, payment_method,
    payment_reference, reservation_expires_at, paid_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: CreateParticipant :one
INSERT INTO participants (event_id, booking_id, name, email, newsletter_optin, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreateTicket :one
-- id is supplied by the caller so qr_token (HMAC over ticket id + nonce)
-- can be precomputed before the INSERT.
INSERT INTO tickets (
    id, booking_id, participant_id, event_id, status, qr_token, qr_nonce,
    phase_id, category_id, duration_id, amount_minor, paid_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: CountActiveSeatsForEvent :one
-- Counts tickets that consume a seat. Canceled tickets do not consume.
-- BOOKED tickets count even if reservation isn't paid yet — capacity must
-- account for in-flight bookings or two parallel bookings could both succeed
-- past the limit.
SELECT count(*)::int AS seats
FROM tickets
WHERE event_id = $1 AND status IN ('booked', 'paid', 'checked_in');

-- name: CountActiveSeatsForEventLocking :one
-- Same as above but takes a row-level lock on the events row first to
-- serialize concurrent bookings. Used inside the booking transaction.
WITH locked AS (
    SELECT events.id AS event_id FROM events WHERE events.id = $1 FOR UPDATE
)
SELECT count(*)::int AS seats
FROM tickets, locked
WHERE tickets.event_id = locked.event_id
  AND tickets.status IN ('booked', 'paid', 'checked_in');

-- name: ListExpiredUnpaidBookings :many
-- Returns booking ids that have status='booked' and reservation_expires_at
-- in the past. Sweeper invokes this and then cascades to ticket cancel.
SELECT id, event_id FROM bookings
WHERE status = 'booked'
  AND reservation_expires_at IS NOT NULL
  AND reservation_expires_at < now();

-- name: CancelBookingAndTickets :exec
-- Marks the booking and all its tickets as canceled in one statement.
-- Used by the sweeper and by ticket-cancel handlers in Phase 5.
WITH booking_update AS (
    UPDATE bookings SET status = 'canceled', canceled_at = now()
    WHERE bookings.id = $1 AND bookings.status <> 'canceled'
    RETURNING bookings.id AS booking_id
)
UPDATE tickets SET status = 'canceled', canceled_at = now()
WHERE tickets.booking_id = (SELECT booking_id FROM booking_update);

-- name: GetBookingByID :one
SELECT * FROM bookings WHERE id = $1;

-- name: ListParticipantsForBooking :many
SELECT * FROM participants WHERE booking_id = $1 ORDER BY created_at;

-- name: MarkBookingPaid :exec
UPDATE bookings
SET status = 'paid',
    paid_at = now(),
    payment_method = COALESCE(payment_method, $2)
WHERE id = $1 AND status = 'booked';

-- name: MarkBookingTicketsPaid :exec
UPDATE tickets
SET status = 'paid', paid_at = now()
WHERE booking_id = $1 AND status = 'booked';

-- name: PurgeTestBookingsForEvent :execrows
-- Deletes all bookings on an event that were created in test mode
-- (payment_method='test'). Cascade deletes participants + tickets + redemptions.
DELETE FROM bookings WHERE event_id = $1 AND payment_method = 'test';

-- name: ListBookingsForEvent :many
-- Kept for backwards compat with code that doesn't yet use the filtered
-- variant. New callers should use SearchBookingsForEvent.
SELECT b.id, b.event_id, b.contact_email, b.contact_name, b.contact_phone,
       b.locale, b.status, b.total_amount_minor, b.currency, b.coupon_id,
       b.payment_method, b.payment_reference, b.reservation_expires_at,
       b.created_at, b.paid_at, b.canceled_at,
       COUNT(p.id)::int AS participant_count
FROM bookings b
LEFT JOIN participants p ON p.booking_id = b.id
WHERE b.event_id = $1
GROUP BY b.id
ORDER BY b.created_at DESC;

-- name: SearchBookingsForEvent :many
-- Paginated + filterable admin list. Empty filter args are NULL — the WHERE
-- clauses treat NULL as "no filter on this column". Reference search uses the
-- first 8 hex chars of the booking UUID (matches formatReference() output).
-- Search across email + name uses trigram-indexed lower() ILIKE.
SELECT b.id, b.event_id, b.contact_email, b.contact_name, b.contact_phone,
       b.locale, b.status, b.total_amount_minor, b.currency, b.coupon_id,
       b.payment_method, b.payment_reference, b.reservation_expires_at,
       b.created_at, b.paid_at, b.canceled_at,
       COUNT(p.id)::int AS participant_count
FROM bookings b
LEFT JOIN participants p ON p.booking_id = b.id
WHERE b.event_id = $1
  AND (sqlc.narg('status')::text IS NULL OR b.status = sqlc.narg('status')::text)
  AND (sqlc.narg('payment_method')::text IS NULL OR b.payment_method = sqlc.narg('payment_method')::text)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR b.created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz   IS NULL OR b.created_at <  sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('q')::text IS NULL OR
       lower(b.contact_email) LIKE '%' || lower(sqlc.narg('q')::text) || '%' OR
       lower(b.contact_name)  LIKE '%' || lower(sqlc.narg('q')::text) || '%' OR
       upper(substring(replace(b.id::text, '-', '') from 1 for 8)) LIKE upper('%' || sqlc.narg('q')::text || '%'))
GROUP BY b.id
ORDER BY
    CASE WHEN sqlc.arg('sort')::text = 'paid_at_desc'      THEN b.paid_at      END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort')::text = 'paid_at_asc'       THEN b.paid_at      END ASC  NULLS LAST,
    CASE WHEN sqlc.arg('sort')::text = 'total_desc'        THEN b.total_amount_minor END DESC,
    CASE WHEN sqlc.arg('sort')::text = 'total_asc'         THEN b.total_amount_minor END ASC,
    CASE WHEN sqlc.arg('sort')::text = 'name_asc'          THEN lower(b.contact_name) END ASC,
    CASE WHEN sqlc.arg('sort')::text = 'created_at_asc'    THEN b.created_at END ASC,
    b.created_at DESC
LIMIT sqlc.arg('limit_n')::int OFFSET sqlc.arg('offset_n')::int;

-- name: CountSearchBookingsForEvent :one
-- Matches SearchBookingsForEvent filters minus pagination/sort. Used for the
-- total row count and the summary strip.
-- Revenue formula (real money received, not "suggested totals"):
--
--   pre_paid_revenue  = SUM(booking.total_amount_minor) for paid bookings
--                       whose payment_method is NOT in ('donation', 'at_door')
--                       — these were collected via bank transfer / PayPal /
--                         test mode upfront, so the booking total IS the
--                         money received.
--
--   door_revenue      = SUM(ticket.paid_amount_minor) over the tickets of
--                       paid bookings — populated by the door scanner at
--                       check-in. For at-door / donation bookings this is
--                       the ONLY source of real money (booking.total_minor
--                       was the suggested / expected amount, not what
--                       actually got handed over). For pre-paid bookings
--                       the column stays NULL and contributes nothing.
--
--   revenue_minor     = pre_paid_revenue + door_revenue
--
-- The participant_count subquery is duplicated rather than joined to avoid
-- multiplying the per-booking sums by the participant count.
SELECT
    count(*)::int                                                    AS total_count,
    count(*) FILTER (WHERE b.status = 'paid')::int                   AS paid_count,
    count(*) FILTER (WHERE b.status = 'booked')::int                 AS booked_count,
    count(*) FILTER (WHERE b.status = 'canceled')::int               AS canceled_count,
    COALESCE(sum(
        CASE
            WHEN b.status = 'paid'
                AND COALESCE(b.payment_method, '') NOT IN ('donation', 'at_door')
            THEN b.total_amount_minor
            ELSE 0
        END
        +
        CASE
            WHEN b.status = 'paid'
            THEN COALESCE((
                SELECT SUM(t.paid_amount_minor)::bigint
                FROM tickets t
                WHERE t.booking_id = b.id AND t.paid_amount_minor IS NOT NULL
            ), 0)
            ELSE 0
        END
    ), 0)::bigint AS paid_revenue_minor,
    COALESCE(sum((SELECT count(*) FROM participants WHERE booking_id = b.id)::int), 0)::int                                   AS total_participants,
    COALESCE(sum((SELECT count(*) FROM participants WHERE booking_id = b.id)::int) FILTER (WHERE b.status = 'paid'), 0)::int  AS paid_participants,
    COALESCE(sum((SELECT count(*) FROM participants WHERE booking_id = b.id)::int) FILTER (WHERE b.status = 'booked'), 0)::int AS booked_participants
FROM bookings b
WHERE b.event_id = $1
  AND (sqlc.narg('status')::text IS NULL OR b.status = sqlc.narg('status')::text)
  AND (sqlc.narg('payment_method')::text IS NULL OR b.payment_method = sqlc.narg('payment_method')::text)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR b.created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz   IS NULL OR b.created_at <  sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('q')::text IS NULL OR
       lower(b.contact_email) LIKE '%' || lower(sqlc.narg('q')::text) || '%' OR
       lower(b.contact_name)  LIKE '%' || lower(sqlc.narg('q')::text) || '%' OR
       upper(substring(replace(b.id::text, '-', '') from 1 for 8)) LIKE upper('%' || sqlc.narg('q')::text || '%'));

-- name: UpdateBookingContactEmail :exec
-- Narrow PATCH endpoint for typo fixes. Returns no rows; the handler re-loads
-- if it needs to echo the updated state back. Audit-log the old/new values
-- at the handler level (the row update is the source of truth for the new
-- value).
UPDATE bookings SET contact_email = $2 WHERE id = $1;

-- name: ListExpandedBookingsForEvent :many
-- CSV export. Same filter contract as SearchBookingsForEvent but without
-- pagination — caller streams. Participant names are aggregated into a
-- pipe-separated string so each row stays atomic in the CSV.
SELECT b.id, b.contact_email, b.contact_name,
       b.status, b.total_amount_minor, b.currency,
       b.payment_method, b.paid_at, b.created_at,
       COUNT(p.id)::int AS participant_count,
       COALESCE(string_agg(p.name, '|' ORDER BY p.created_at), '')::text AS participant_names
FROM bookings b
LEFT JOIN participants p ON p.booking_id = b.id
WHERE b.event_id = $1
  AND (sqlc.narg('status')::text IS NULL OR b.status = sqlc.narg('status')::text)
  AND (sqlc.narg('payment_method')::text IS NULL OR b.payment_method = sqlc.narg('payment_method')::text)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR b.created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz   IS NULL OR b.created_at <  sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('q')::text IS NULL OR
       lower(b.contact_email) LIKE '%' || lower(sqlc.narg('q')::text) || '%' OR
       lower(b.contact_name)  LIKE '%' || lower(sqlc.narg('q')::text) || '%' OR
       upper(substring(replace(b.id::text, '-', '') from 1 for 8)) LIKE upper('%' || sqlc.narg('q')::text || '%'))
GROUP BY b.id
ORDER BY b.created_at ASC;
