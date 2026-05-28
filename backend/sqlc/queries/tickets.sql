-- name: GetTicketByID :one
SELECT * FROM tickets WHERE id = $1;

-- name: GetTicketWithContext :one
-- Joins everything the public ticket page (and the door scanner) needs in
-- one query. booking_payment_method drives the scanner's "ask for door
-- cash" decision: ('donation', 'at_door') → show amount input.
SELECT
    t.id, t.booking_id, t.participant_id, t.event_id, t.status,
    t.qr_token, t.qr_nonce, t.amount_minor, t.paid_amount_minor,
    t.created_at, t.paid_at, t.canceled_at, t.checked_in_at,
    p.name AS participant_name, p.email AS participant_email,
    e.name AS event_name, e.slug AS event_slug,
    e.starts_at AS event_starts_at, e.ends_at AS event_ends_at,
    e.location AS event_location, e.currency AS event_currency,
    e.color_primary AS event_color_primary,
    e.color_secondary AS event_color_secondary,
    e.color_text AS event_color_text,
    e.pricing_mode AS event_pricing_mode,
    e.payment_timing AS event_payment_timing,
    ph.name AS phase_name,
    cat.name AS category_name,
    dur.name AS duration_name,
    b.contact_email AS booking_contact_email,
    b.payment_method AS booking_payment_method
FROM tickets t
JOIN participants p ON p.id = t.participant_id
JOIN events e ON e.id = t.event_id
JOIN bookings b ON b.id = t.booking_id
LEFT JOIN price_phases ph ON ph.id = t.phase_id
LEFT JOIN price_categories cat ON cat.id = t.category_id
LEFT JOIN price_durations dur ON dur.id = t.duration_id
WHERE t.id = $1;

-- name: CancelTicketByID :exec
-- Only cancels tickets currently in 'booked' or 'paid' state. CHECKED_IN
-- tickets cannot be canceled by the holder; canceled tickets are a no-op.
UPDATE tickets
SET status = 'canceled', canceled_at = now()
WHERE id = $1 AND status IN ('booked', 'paid');

-- name: TransferTicket :exec
-- Atomic transfer: rotates qr_nonce and qr_token, plus updates the
-- participant row that's joined to this ticket via participant_id.
UPDATE tickets
SET qr_nonce = $2, qr_token = $3
WHERE id = $1;

-- name: UpdateParticipantContact :exec
UPDATE participants
SET name = $2, email = $3
WHERE id = $1;

-- name: ListActiveTicketsForBooking :many
SELECT * FROM tickets
WHERE booking_id = $1 AND status IN ('booked', 'paid', 'checked_in');

-- name: CheckInTicket :one
-- Idempotent door-scan. If the ticket is already checked_in we return the
-- existing row unchanged (caller compares status before vs. after to decide
-- whether to show "✓ checked in just now" or "⚠ already checked in at X").
-- Only paid or already-checked-in tickets can be scanned; booked/canceled
-- tickets don't match and the caller surfaces the right error.
--
-- $2 is the door-collected amount: 0 / NULL for pre-paid tickets, a real
-- number for at_door / donation tickets. Captured ONLY on the first scan —
-- re-scans don't overwrite the previously recorded amount (COALESCE on the
-- target column). To correct a mistyped amount, undo first then re-scan.
UPDATE tickets
SET status = 'checked_in',
    checked_in_at = COALESCE(checked_in_at, now()),
    paid_amount_minor = COALESCE(paid_amount_minor, sqlc.narg('paid_amount_minor')::bigint)
WHERE id = $1
  AND status IN ('paid', 'checked_in')
RETURNING *;

-- name: UndoCheckInTicket :one
-- Reverses an accidental check-in: clears checked_in_at + paid_amount_minor
-- and flips status back to 'paid'. Audit-logged at the handler. Only works
-- on currently-checked-in tickets (no-op otherwise).
UPDATE tickets
SET status = 'paid',
    checked_in_at = NULL,
    paid_amount_minor = NULL
WHERE id = $1
  AND status = 'checked_in'
RETURNING *;

-- name: SearchActiveTicketsForEvent :many
-- Manual check-in fallback. Accepts a free-text query and ILIKE-matches
-- across participant name + email, booking contact name + email, AND the
-- booking-reference prefix (first 8 hex of the booking UUID). Scoped to
-- the event so a manager can't accidentally pull tickets from another
-- event with overlapping data.
--
-- One row per *participant* (= per ticket) — the scanner lists them and
-- the staffer taps a single person to check in. LIMIT 25 caps the
-- response; typing more characters narrows the list.
SELECT
    t.id, t.status, t.checked_in_at,
    p.name AS participant_name,
    p.email AS participant_email,
    b.id AS booking_id,
    b.contact_name AS contact_name,
    b.contact_email AS contact_email
FROM tickets t
JOIN participants p ON p.id = t.participant_id
JOIN bookings b ON b.id = t.booking_id
WHERE t.event_id = $1
  AND t.status IN ('paid', 'checked_in')
  AND (
       lower(p.name) LIKE '%' || lower($2::text) || '%'
    OR lower(b.contact_name) LIKE '%' || lower($2::text) || '%'
    OR lower(COALESCE(p.email, '')) LIKE '%' || lower($2::text) || '%'
    OR lower(b.contact_email) LIKE '%' || lower($2::text) || '%'
    OR upper(substring(replace(b.id::text, '-', '') from 1 for 8)) LIKE upper('%' || $2::text || '%')
  )
ORDER BY p.name ASC
LIMIT 25;

-- name: CountCheckedInForEvent :one
SELECT
    count(*) FILTER (WHERE status IN ('paid', 'checked_in'))::int AS expected,
    count(*) FILTER (WHERE status = 'checked_in')::int            AS checked_in
FROM tickets
WHERE event_id = $1;

-- name: ListRecentCheckInsForEvent :many
-- Most recent N check-ins, used by the scanner page to show a live log so
-- door staff can spot mistakes ("oh I scanned the wrong person"). Includes
-- the recorded paid_amount_minor (= cash collected at the door) for the
-- few-row "Rückgängig" affordance.
SELECT
    t.id, t.checked_in_at, t.paid_amount_minor,
    p.name AS participant_name,
    p.email AS participant_email,
    upper(substring(replace(b.id::text, '-', '') from 1 for 8)) AS booking_reference
FROM tickets t
JOIN participants p ON p.id = t.participant_id
JOIN bookings b ON b.id = t.booking_id
WHERE t.event_id = $1 AND t.status = 'checked_in' AND t.checked_in_at IS NOT NULL
ORDER BY t.checked_in_at DESC
LIMIT $2;
