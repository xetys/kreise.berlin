-- +goose Up

-- PayPal.me handle: just the username segment of paypal.me/<handle>. The
-- frontend renders deep-links of the form paypal.me/<handle>/<amount>EUR.
-- No API integration; admin still confirms via mark-paid.
ALTER TABLE events ADD COLUMN paypal_handle TEXT;

-- Per-event test-mode flag. When true, bookings on this event auto-flip
-- to status='paid' immediately without going through PayPal/bank, and are
-- tagged with payment_method='test' so admins can clean them up before
-- going live.
ALTER TABLE events ADD COLUMN payment_test_mode BOOLEAN NOT NULL DEFAULT false;

-- Allow the new 'test' payment_method.
ALTER TABLE bookings DROP CONSTRAINT bookings_payment_method_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_payment_method_check CHECK (
    payment_method IN ('paypal', 'bank_transfer', 'donation', 'at_door', 'test')
    OR payment_method IS NULL
);

-- +goose Down

ALTER TABLE bookings DROP CONSTRAINT bookings_payment_method_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_payment_method_check CHECK (
    payment_method IN ('paypal', 'bank_transfer', 'donation', 'at_door')
    OR payment_method IS NULL
);

ALTER TABLE events DROP COLUMN payment_test_mode;
ALTER TABLE events DROP COLUMN paypal_handle;
