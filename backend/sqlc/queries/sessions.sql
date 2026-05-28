-- name: CreateSession :one
INSERT INTO sessions (id, user_id, expires_at, user_agent, ip, password_version, impersonator_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSessionByID :one
-- The LEFT JOIN on the impersonator pulls the original actor's email when
-- the session was minted via /admin/users/{id}/impersonate. Frontend uses
-- this to show the "Du arbeitest gerade als …" banner; the impersonator's
-- email never affects authorization (the middleware authorizes against
-- u.role / u.id only).
SELECT
    s.id,
    s.user_id,
    s.created_at,
    s.last_seen_at,
    s.expires_at,
    s.revoked_at,
    s.user_agent,
    s.ip,
    s.password_version AS session_password_version,
    s.impersonator_user_id,
    u.email            AS user_email,
    u.role             AS user_role,
    u.display_name     AS user_display_name,
    u.disabled_at      AS user_disabled_at,
    u.active           AS user_active,
    u.password_version AS user_password_version,
    imp.email          AS impersonator_email
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN users imp ON imp.id = s.impersonator_user_id
WHERE s.id = $1;

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE sessions
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at < now() OR revoked_at < now() - INTERVAL '7 days';
