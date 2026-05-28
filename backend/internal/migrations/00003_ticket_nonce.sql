-- +goose Up

-- qr_nonce backs both the QR token (used at the door for check-in) and the
-- view token (used in the magic-link ticket-page URL). Rotating the nonce
-- on transfer invalidates the old holder's links atomically.
ALTER TABLE tickets ADD COLUMN qr_nonce BYTEA;
UPDATE tickets SET qr_nonce = gen_random_bytes(16) WHERE qr_nonce IS NULL;
ALTER TABLE tickets ALTER COLUMN qr_nonce SET NOT NULL;

-- +goose Down

ALTER TABLE tickets DROP COLUMN qr_nonce;
