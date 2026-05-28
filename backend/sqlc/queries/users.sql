-- name: CreateUser :one
INSERT INTO users (email, password_hash, role, display_name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE lower(email) = lower($1) AND disabled_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1;

-- name: DisableUser :exec
UPDATE users SET disabled_at = now() WHERE id = $1 AND disabled_at IS NULL;

-- name: UpdateUserPassword :exec
-- Updates the password hash AND bumps password_version, which kills every
-- outstanding session for the user (sessions remember the version they were
-- minted at; the auth middleware rejects mismatches).
UPDATE users SET password_hash = $2, password_version = password_version + 1 WHERE id = $1;

-- name: ListUsers :many
SELECT id, email, role, display_name, active, created_at, disabled_at, last_login_at
FROM users
ORDER BY created_at DESC;

-- name: CountGlobalAdmins :one
SELECT count(*)::int AS n
FROM users
WHERE role = 'global_admin' AND disabled_at IS NULL;

-- name: SetUserActive :exec
UPDATE users SET active = $2 WHERE id = $1;

-- name: ReactivateUser :exec
UPDATE users SET disabled_at = NULL, active = true WHERE id = $1;

-- name: SetUserRole :exec
UPDATE users SET role = $2 WHERE id = $1;

-- name: BumpPasswordVersion :exec
-- Force-logout: bump the user's password_version without touching the hash.
UPDATE users SET password_version = password_version + 1 WHERE id = $1;

-- name: SetUserPasswordAndActivate :exec
-- One-shot used by the setup-token flow: writes the new hash, marks the
-- user active, bumps password_version (so any pre-setup session is killed).
UPDATE users
SET password_hash = $2, active = true, password_version = password_version + 1
WHERE id = $1;

-- name: TouchUserLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: UpdateUserDisplayName :exec
UPDATE users SET display_name = $2 WHERE id = $1;

-- name: CountSessionsForUser :one
SELECT count(*)::int AS n FROM sessions WHERE user_id = $1 AND expires_at > now();

-- name: CreateInvitedUser :one
-- Used by the invite flow. password_hash is a placeholder (random bytes,
-- argon2id-hashed). active=false until the invitee completes setup.
INSERT INTO users (email, password_hash, role, display_name, active)
VALUES ($1, $2, $3, $4, false)
RETURNING *;

-- name: CreateSetupToken :one
INSERT INTO setup_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSetupTokenByHash :one
SELECT * FROM setup_tokens
WHERE token_hash = $1
  AND used_at IS NULL
  AND expires_at > now();

-- name: ConsumeSetupToken :exec
UPDATE setup_tokens SET used_at = now() WHERE id = $1;

-- name: InvalidateOutstandingSetupTokensForUser :exec
-- Marks every still-valid setup token for a user as used. Called when an admin
-- resends the invite so the previous link stops working.
UPDATE setup_tokens
SET used_at = now()
WHERE user_id = $1 AND used_at IS NULL;

-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordResetTokenByHash :one
SELECT * FROM password_reset_tokens
WHERE token_hash = $1
  AND used_at IS NULL
  AND expires_at > now();

-- name: ConsumePasswordResetToken :exec
UPDATE password_reset_tokens SET used_at = now() WHERE id = $1;
