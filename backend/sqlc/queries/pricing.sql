-- ============================================================================
-- Phases
-- ============================================================================

-- name: ListPricePhases :many
SELECT * FROM price_phases WHERE event_id = $1 ORDER BY ordering, starts_at;

-- name: GetPricePhase :one
SELECT * FROM price_phases WHERE id = $1;

-- name: GetActivePhaseForEvent :one
-- Returns the phase whose [starts_at, ends_at) contains $2.
-- If multiple phases overlap, the lowest `ordering` wins; ties broken by id.
SELECT * FROM price_phases
WHERE event_id = $1
  AND starts_at <= $2 AND ends_at > $2
ORDER BY ordering, id
LIMIT 1;

-- name: CreatePricePhase :one
INSERT INTO price_phases (event_id, name, starts_at, ends_at, ordering)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdatePricePhase :one
UPDATE price_phases SET
    name = $2, starts_at = $3, ends_at = $4, ordering = $5
WHERE id = $1
RETURNING *;

-- name: DeletePricePhase :exec
DELETE FROM price_phases WHERE id = $1;

-- ============================================================================
-- Categories
-- ============================================================================

-- name: ListPriceCategories :many
SELECT * FROM price_categories WHERE event_id = $1 ORDER BY ordering, name;

-- name: GetPriceCategory :one
SELECT * FROM price_categories WHERE id = $1;

-- name: CreatePriceCategory :one
INSERT INTO price_categories (event_id, name, ordering)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdatePriceCategory :one
UPDATE price_categories SET name = $2, ordering = $3 WHERE id = $1 RETURNING *;

-- name: DeletePriceCategory :exec
DELETE FROM price_categories WHERE id = $1;

-- ============================================================================
-- Durations
-- ============================================================================

-- name: ListPriceDurations :many
SELECT * FROM price_durations WHERE event_id = $1 ORDER BY ordering, name;

-- name: GetPriceDuration :one
SELECT * FROM price_durations WHERE id = $1;

-- name: CreatePriceDuration :one
INSERT INTO price_durations (event_id, name, ordering)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdatePriceDuration :one
UPDATE price_durations SET name = $2, ordering = $3 WHERE id = $1 RETURNING *;

-- name: DeletePriceDuration :exec
DELETE FROM price_durations WHERE id = $1;

-- ============================================================================
-- Prices (sparse matrix cells)
-- ============================================================================

-- name: ListPrices :many
SELECT * FROM prices WHERE event_id = $1;

-- name: GetPriceFor :one
-- Looks up a price by (phase, category, duration). duration_id may be NULL
-- in both query and stored row (events without duration dimension).
-- IS NOT DISTINCT FROM is the null-safe equality operator.
SELECT * FROM prices
WHERE phase_id = $1
  AND category_id = $2
  AND duration_id IS NOT DISTINCT FROM sqlc.narg('duration_id')
LIMIT 1;

-- name: UpsertPrice :one
INSERT INTO prices (event_id, phase_id, category_id, duration_id, amount_minor)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (phase_id, category_id, duration_id)
DO UPDATE SET amount_minor = EXCLUDED.amount_minor
RETURNING *;

-- name: DeletePrice :exec
DELETE FROM prices WHERE id = $1;

-- ============================================================================
-- Donation config
-- ============================================================================

-- name: GetDonationConfig :one
SELECT * FROM donation_configs WHERE event_id = $1;

-- name: UpsertDonationConfig :one
INSERT INTO donation_configs (event_id, suggested_minor, min_minor)
VALUES ($1, $2, $3)
ON CONFLICT (event_id) DO UPDATE
SET suggested_minor = EXCLUDED.suggested_minor, min_minor = EXCLUDED.min_minor
RETURNING *;

-- name: DeleteDonationConfig :exec
DELETE FROM donation_configs WHERE event_id = $1;

-- ============================================================================
-- Coupons
-- ============================================================================

-- name: ListCoupons :many
SELECT * FROM coupons WHERE event_id = $1 ORDER BY created_at;

-- name: GetCoupon :one
SELECT * FROM coupons WHERE id = $1;

-- name: GetCouponByCode :one
SELECT * FROM coupons WHERE event_id = $1 AND code = $2;

-- name: GetCouponByCodeForUpdate :one
-- Locks the coupon row inside a transaction so concurrent bookings can't
-- both succeed past max_uses.
SELECT * FROM coupons WHERE event_id = $1 AND code = $2 FOR UPDATE;

-- name: CreateCoupon :one
INSERT INTO coupons (
    event_id, code, type, value_minor, value_percent,
    max_uses, valid_from, valid_to, single_use_per_email
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateCoupon :one
UPDATE coupons SET
    type = $2,
    value_minor = $3,
    value_percent = $4,
    max_uses = $5,
    valid_from = $6,
    valid_to = $7,
    single_use_per_email = $8
WHERE id = $1
RETURNING *;

-- name: DeleteCoupon :exec
DELETE FROM coupons WHERE id = $1;

-- name: ListCouponPhaseFilters :many
SELECT phase_id FROM coupon_phase_filters WHERE coupon_id = $1;

-- name: ListCouponCategoryFilters :many
SELECT category_id FROM coupon_category_filters WHERE coupon_id = $1;

-- name: AddCouponPhaseFilter :exec
INSERT INTO coupon_phase_filters (coupon_id, phase_id)
VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ClearCouponPhaseFilters :exec
DELETE FROM coupon_phase_filters WHERE coupon_id = $1;

-- name: AddCouponCategoryFilter :exec
INSERT INTO coupon_category_filters (coupon_id, category_id)
VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ClearCouponCategoryFilters :exec
DELETE FROM coupon_category_filters WHERE coupon_id = $1;

-- name: CountCouponRedemptions :one
SELECT count(*)::int AS uses FROM coupon_redemptions WHERE coupon_id = $1;

-- name: CountCouponRedemptionsForEmail :one
-- Counts how many bookings using contact_email have redeemed this coupon
-- (used for single_use_per_email enforcement).
SELECT count(*)::int AS uses
FROM coupon_redemptions cr
JOIN bookings b ON b.id = cr.booking_id
WHERE cr.coupon_id = $1 AND lower(b.contact_email) = lower($2);

-- name: RecordCouponRedemption :exec
INSERT INTO coupon_redemptions (coupon_id, booking_id) VALUES ($1, $2);
