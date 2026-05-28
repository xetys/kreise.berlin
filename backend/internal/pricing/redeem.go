package pricing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// RedeemCoupon records a coupon redemption inside the booking transaction.
//
// Caller passes a *db.Queries already bound to a transaction (via
// database.Pool.WithTx). This function takes a row-level lock on the coupon
// (SELECT … FOR UPDATE) and rechecks max_uses + single_use_per_email under
// the lock, so concurrent bookings cannot exceed limits.
//
// On success, a coupon_redemptions row is inserted. On any eligibility
// failure under the lock, the corresponding sentinel error is returned and
// the caller MUST roll back the transaction.
func RedeemCoupon(
	ctx context.Context,
	q *db.Queries,
	eventID, bookingID uuid.UUID,
	code string,
	contactEmail string,
) error {
	row, err := q.GetCouponByCodeForUpdate(ctx, db.GetCouponByCodeForUpdateParams{
		EventID: eventID,
		Code:    code,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCouponNotFound
		}
		return fmt.Errorf("lock coupon: %w", err)
	}

	if row.MaxUses != nil {
		used, err := q.CountCouponRedemptions(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("count redemptions: %w", err)
		}
		if int(used) >= int(*row.MaxUses) {
			return ErrCouponMaxUsesExceeded
		}
	}

	if row.SingleUsePerEmail && contactEmail != "" {
		used, err := q.CountCouponRedemptionsForEmail(ctx, db.CountCouponRedemptionsForEmailParams{
			CouponID: row.ID,
			Lower:    contactEmail,
		})
		if err != nil {
			return fmt.Errorf("count redemptions for email: %w", err)
		}
		if int(used) > 0 {
			return ErrCouponAlreadyUsed
		}
	}

	if err := q.RecordCouponRedemption(ctx, db.RecordCouponRedemptionParams{
		CouponID:  row.ID,
		BookingID: bookingID,
	}); err != nil {
		return fmt.Errorf("insert redemption: %w", err)
	}
	return nil
}
