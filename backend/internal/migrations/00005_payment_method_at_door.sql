-- +goose Up

-- Phase 6 introduced the 'at_door' payment_method (matrix events with
-- payment_timing='at_door' get marked paid immediately and tracked as
-- 'at_door' so admins can tell them apart from PayPal / bank transfer in
-- the bookings dashboard).
ALTER TABLE bookings DROP CONSTRAINT bookings_payment_method_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_payment_method_check CHECK (
    payment_method IN ('paypal', 'bank_transfer', 'donation', 'at_door')
    OR payment_method IS NULL
);

-- +goose Down

ALTER TABLE bookings DROP CONSTRAINT bookings_payment_method_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_payment_method_check CHECK (
    payment_method IN ('paypal', 'bank_transfer', 'donation')
    OR payment_method IS NULL
);
