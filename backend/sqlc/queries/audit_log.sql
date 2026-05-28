-- name: InsertAuditLog :exec
INSERT INTO audit_log (actor_user_id, event_id, action, target_kind, target_id, payload)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAuditForEvent :many
SELECT * FROM audit_log
WHERE event_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
