-- name: GetEventByID :one
SELECT * FROM events WHERE id = $1;

-- name: GetEventBySlug :one
SELECT * FROM events WHERE slug = $1;

-- name: SlugTaken :one
SELECT EXISTS (SELECT 1 FROM events WHERE slug = $1);

-- name: ListPublicUpcomingEvents :many
SELECT * FROM events
WHERE is_public = TRUE
  AND archived_at IS NULL
  AND ends_at > now()
ORDER BY starts_at ASC;

-- name: ListEventsForGlobalAdmin :many
SELECT * FROM events
ORDER BY starts_at DESC;

-- name: ListEventsForUser :many
-- Events the user can see as event_admin OR event_manager.
SELECT DISTINCT e.* FROM events e
LEFT JOIN event_admins ea ON ea.event_id = e.id AND ea.user_id = $1
LEFT JOIN event_managers em ON em.event_id = e.id AND em.user_id = $1
WHERE ea.user_id IS NOT NULL OR em.user_id IS NOT NULL
ORDER BY e.starts_at DESC;

-- name: IsEventAdmin :one
SELECT EXISTS (
    SELECT 1 FROM event_admins
    WHERE event_id = $1 AND user_id = $2
);

-- name: IsEventManager :one
SELECT EXISTS (
    SELECT 1 FROM event_managers
    WHERE event_id = $1 AND user_id = $2
);

-- name: CreateEvent :one
INSERT INTO events (
    slug, name, description, banner_ref,
    color_primary, color_secondary, color_text,
    location, starts_at, ends_at, participant_limit,
    pricing_mode, currency, default_locale,
    is_public, created_by,
    payment_timing, bank_iban, bank_bic, bank_account_holder,
    paypal_handle, payment_test_mode
)
VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14,
    $15, $16,
    $17, $18, $19, $20,
    $21, $22
)
RETURNING *;

-- name: UpdateEvent :one
UPDATE events SET
    name = $2,
    description = $3,
    color_primary = $4,
    color_secondary = $5,
    color_text = $6,
    location = $7,
    starts_at = $8,
    ends_at = $9,
    participant_limit = $10,
    pricing_mode = $11,
    currency = $12,
    default_locale = $13,
    payment_timing = $14,
    bank_iban = $15,
    bank_bic = $16,
    bank_account_holder = $17,
    paypal_handle = $18,
    payment_test_mode = $19
WHERE id = $1
RETURNING *;

-- name: ArchiveEvent :exec
UPDATE events SET archived_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: UnarchiveEvent :exec
UPDATE events SET archived_at = NULL WHERE id = $1;

-- name: PublishEvent :exec
UPDATE events SET is_public = TRUE WHERE id = $1;

-- name: UnpublishEvent :exec
UPDATE events SET is_public = FALSE WHERE id = $1;

-- name: SetEventBanner :exec
UPDATE events SET banner_ref = $2 WHERE id = $1;
