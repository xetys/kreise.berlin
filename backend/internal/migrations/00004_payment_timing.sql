-- +goose Up

-- Per-event payment timing. Donation events ignore this (their payment is
-- the donation amount itself, collected at booking or at the door — there's
-- no separate gate). For matrix events:
--   beforehand → status=booked at first; admin marks paid OR PayPal capture flips to paid; QR follows.
--   at_door     → status=paid immediately on booking; QR available right away; money collected on arrival.
ALTER TABLE events
    ADD COLUMN payment_timing TEXT NOT NULL DEFAULT 'beforehand'
        CHECK (payment_timing IN ('beforehand', 'at_door'));

-- Bank-transfer info, displayed in the Stage 1 email + on the booked-confirmation
-- page. Optional: events that don't accept bank transfer can leave these empty.
ALTER TABLE events ADD COLUMN bank_iban TEXT;
ALTER TABLE events ADD COLUMN bank_bic TEXT;
ALTER TABLE events ADD COLUMN bank_account_holder TEXT;

-- +goose Down

ALTER TABLE events DROP COLUMN bank_account_holder;
ALTER TABLE events DROP COLUMN bank_bic;
ALTER TABLE events DROP COLUMN bank_iban;
ALTER TABLE events DROP COLUMN payment_timing;
