-- name: AssignEventAdmin :exec
INSERT INTO event_admins (user_id, event_id)
VALUES ($1, $2)
ON CONFLICT (user_id, event_id) DO NOTHING;

-- name: UnassignEventAdmin :exec
DELETE FROM event_admins WHERE user_id = $1 AND event_id = $2;

-- name: AssignEventManager :exec
INSERT INTO event_managers (user_id, event_id)
VALUES ($1, $2)
ON CONFLICT (user_id, event_id) DO NOTHING;

-- name: UnassignEventManager :exec
DELETE FROM event_managers WHERE user_id = $1 AND event_id = $2;

-- name: ListEventAdminUsers :many
SELECT u.id, u.email, u.role, u.display_name, u.disabled_at, ea.assigned_at
FROM event_admins ea
JOIN users u ON u.id = ea.user_id
WHERE ea.event_id = $1
ORDER BY ea.assigned_at;

-- name: ListEventManagerUsers :many
SELECT u.id, u.email, u.role, u.display_name, u.disabled_at, em.assigned_at
FROM event_managers em
JOIN users u ON u.id = em.user_id
WHERE em.event_id = $1
ORDER BY em.assigned_at;
