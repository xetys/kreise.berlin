-- name: ListProgramEntries :many
SELECT * FROM program_entries
WHERE event_id = $1
ORDER BY ordering ASC, starts_at ASC;

-- name: GetProgramEntry :one
SELECT * FROM program_entries WHERE id = $1;

-- name: CreateProgramEntry :one
INSERT INTO program_entries (event_id, starts_at, ends_at, title, description, ordering)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateProgramEntry :one
UPDATE program_entries SET
    starts_at = $2,
    ends_at = $3,
    title = $4,
    description = $5,
    ordering = $6
WHERE id = $1
RETURNING *;

-- name: DeleteProgramEntry :exec
DELETE FROM program_entries WHERE id = $1;
