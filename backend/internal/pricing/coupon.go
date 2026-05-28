package pricing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// applyCoupon resolves the coupon, runs eligibility checks, and reduces the
// quote total accordingly. Mutates q in place.
func applyCoupon(ctx context.Context, repo Repo, in QuoteInput, q *Quote) error {
	coup, err := repo.GetCouponByCode(ctx, in.Event.ID, in.CouponCode)
	if err != nil {
		if errors.Is(err, ErrCouponNotFound) {
			return ErrCouponNotFound
		}
		return fmt.Errorf("get coupon: %w", err)
	}

	if err := checkCouponEligibility(ctx, repo, coup, in, q.LineItems); err != nil {
		return err
	}

	discount := computeDiscount(coup, q.SubtotalMinor)
	if discount > q.SubtotalMinor {
		discount = q.SubtotalMinor
	}
	q.DiscountMinor = discount
	q.TotalMinor = q.SubtotalMinor - discount
	q.AppliedCouponID = &coup.ID
	q.AppliedCouponCode = coup.Code
	return nil
}

func checkCouponEligibility(ctx context.Context, repo Repo, c Coupon, in QuoteInput, items []LineItem) error {
	now := in.When

	if c.ValidFrom != nil && now.Before(*c.ValidFrom) {
		return ErrCouponNotYetValid
	}
	if c.ValidTo != nil && now.After(*c.ValidTo) {
		return ErrCouponExpired
	}

	if c.MaxUses != nil {
		used, err := repo.CountCouponRedemptions(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("count coupon redemptions: %w", err)
		}
		if used >= int(*c.MaxUses) {
			return ErrCouponMaxUsesExceeded
		}
	}

	if c.SingleUsePerEmail && in.ContactEmail != "" {
		used, err := repo.CountCouponRedemptionsForEmail(ctx, c.ID, in.ContactEmail)
		if err != nil {
			return fmt.Errorf("count coupon redemptions for email: %w", err)
		}
		if used > 0 {
			return ErrCouponAlreadyUsed
		}
	}

	phaseFilters, err := repo.ListCouponPhaseFilters(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("list phase filters: %w", err)
	}
	categoryFilters, err := repo.ListCouponCategoryFilters(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("list category filters: %w", err)
	}

	if len(phaseFilters) == 0 && len(categoryFilters) == 0 {
		return nil
	}

	// applies-to filters: every line item must satisfy each set filter list.
	for i, li := range items {
		if len(phaseFilters) > 0 {
			if li.PhaseID == nil || !containsUUID(phaseFilters, *li.PhaseID) {
				return fmt.Errorf("%w: line item %d phase not in coupon's allowed phases", ErrCouponNotApplicable, i)
			}
		}
		if len(categoryFilters) > 0 {
			if li.CategoryID == nil || !containsUUID(categoryFilters, *li.CategoryID) {
				return fmt.Errorf("%w: line item %d category not in coupon's allowed categories", ErrCouponNotApplicable, i)
			}
		}
	}
	return nil
}

// computeDiscount produces the discount amount in minor units for a coupon
// applied to subtotal. Never negative; never larger than subtotal.
func computeDiscount(c Coupon, subtotal AmountMinor) AmountMinor {
	switch c.Type {
	case CouponFixedReduce:
		if c.ValueMinor == nil {
			return 0
		}
		return AmountMinor(*c.ValueMinor)
	case CouponPercentalReduce:
		if c.ValuePercent == nil {
			return 0
		}
		return percentMinor(subtotal, int64(*c.ValuePercent))
	case CouponGuestlist:
		return subtotal
	default:
		return 0
	}
}

// percentMinor returns round-half-up of (amount * percent / 100).
// Operates only on non-negative amounts (subtotals).
func percentMinor(amount AmountMinor, percent int64) AmountMinor {
	if amount < 0 {
		amount = -amount
	}
	n := int64(amount) * percent
	q := n / 100
	r := n % 100
	if r >= 50 {
		q++
	}
	return AmountMinor(q)
}

func containsUUID(list []uuid.UUID, target uuid.UUID) bool {
	for _, u := range list {
		if u == target {
			return true
		}
	}
	return false
}
