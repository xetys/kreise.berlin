-- +goose Up

-- Bumped on every event that should kill outstanding sessions: password
-- change, deactivate, force-logout. Sessions store the version they were
-- minted at; the auth middleware rejects any session whose version doesn't
-- match the user's current value. Cleaner than a "revoked_sessions" table.
ALTER TABLE users ADD COLUMN password_version INTEGER NOT NULL DEFAULT 1;

-- Add account-active flag (separate from disabled_at — disabled_at says
-- "this user can never log in again", active=false says "invited but not
-- yet completed setup"). password_hash stays NOT NULL for legacy rows; new
-- invited rows fill it with a randomly-generated placeholder that gets
-- overwritten on setup completion.
ALTER TABLE users ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;
-- Last successful login (filled by the auth handler).
ALTER TABLE users ADD COLUMN last_login_at TIMESTAMPTZ;

-- Sessions remember which password_version they were minted at so the
-- middleware can detect a stale session. Default 1 covers any rows from
-- before this migration.
ALTER TABLE sessions ADD COLUMN password_version INTEGER NOT NULL DEFAULT 1;

-- Setup tokens: one-time, hashed at rest. Used by the invite flow to let a
-- newly-invited user complete onboarding (set their own password). Once
-- consumed (used_at) the token cannot be reused.
CREATE TABLE setup_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX setup_tokens_user_idx ON setup_tokens (user_id);

-- Password-reset tokens: same shape, separate table so a leaked setup
-- token cannot reset an existing password (purpose isolation).
CREATE TABLE password_reset_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id);

-- +goose Down

DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS setup_tokens;
ALTER TABLE sessions DROP COLUMN IF EXISTS password_version;
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS active;
ALTER TABLE users DROP COLUMN IF EXISTS password_version;
