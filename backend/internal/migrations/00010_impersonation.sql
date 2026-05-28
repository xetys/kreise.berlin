-- +goose Up

-- Sessions remember who started the impersonation (NULL = normal login).
-- Pure audit / UI signal — does NOT change authz; the effective user is
-- still sessions.user_id and the middleware loads that user's role + scopes.
-- We expose the impersonator's email on /api/auth/me so the frontend can
-- render the "Du arbeitest gerade als X" banner.
ALTER TABLE sessions ADD COLUMN impersonator_user_id UUID
    REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX sessions_impersonator_idx
    ON sessions (impersonator_user_id)
    WHERE impersonator_user_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS sessions_impersonator_idx;
ALTER TABLE sessions DROP COLUMN IF EXISTS impersonator_user_id;
