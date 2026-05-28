-- +goose Up

-- Actual cash collected at door check-in. Differs from tickets.amount_minor
-- (which is the *expected* / suggested amount fixed at booking time):
--
--   - For matrix events with payment_timing = 'beforehand', this stays NULL.
--     The pre-paid amount lives on bookings.total_amount_minor — that's the
--     source of truth for revenue.
--
--   - For matrix events with payment_timing = 'at_door', door staff confirms
--     the amount at check-in (defaults to ticket.amount_minor, can be
--     overridden if e.g. a discount was negotiated).
--
--   - For donation events, the staffer types the real contribution the
--     person handed over at the door. Defaults to the suggested but is
--     usually overwritten.
--
-- Revenue queries treat NULL as "use the pre-paid source"; non-NULL means
-- "door-collected, this is the real number". See
-- CountSearchBookingsForEvent for the combined formula.
ALTER TABLE tickets ADD COLUMN paid_amount_minor BIGINT;

-- +goose Down

ALTER TABLE tickets DROP COLUMN IF EXISTS paid_amount_minor;
