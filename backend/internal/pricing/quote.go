package pricing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

// Input to Quote(). Event is the event being booked. Selections is one entry
// per participant. When is the moment used for phase resolution and coupon
// validity checks (in tests this is fixed; in handlers it's time.Now()).
// CouponCode is empty when no coupon is applied. ContactEmail is needed so
// single_use_per_email coupons can be enforced.
type QuoteInput struct {
	Event        domain.Event
	Selections   []Selection
	When         time.Time
	CouponCode   string
	ContactEmail string
}

// Compute prices the input and returns a Quote. This is the only path that
// produces booking totals; the booking endpoint must call this and never
// accept a client-supplied price.
func Compute(ctx context.Context, repo Repo, in QuoteInput) (Quote, error) {
	if len(in.Selections) == 0 {
		return Quote{}, ErrNoSelections
	}

	q := Quote{Currency: in.Event.Currency}

	switch in.Event.PricingMode {
	case domain.PricingModeMatrix:
		if err := priceMatrix(ctx, repo, in, &q); err != nil {
			return Quote{}, err
		}
	case domain.PricingModeDonation:
		if err := priceDonation(ctx, repo, in, &q); err != nil {
			return Quote{}, err
		}
	default:
		return Quote{}, fmt.Errorf("unknown pricing mode: %s", in.Event.PricingMode)
	}

	q.TotalMinor = q.SubtotalMinor

	if in.CouponCode != "" {
		if err := applyCoupon(ctx, repo, in, &q); err != nil {
			return Quote{}, err
		}
	}

	return q, nil
}

func priceMatrix(ctx context.Context, repo Repo, in QuoteInput, q *Quote) error {
	phase, err := repo.GetActivePhase(ctx, in.Event.ID, in.When)
	if err != nil {
		if errors.Is(err, ErrNoActivePhase) {
			return ErrNoActivePhase
		}
		return fmt.Errorf("get active phase: %w", err)
	}
	q.Phase = &PhaseSnapshot{
		ID: phase.ID, Name: phase.Name, StartsAt: phase.StartsAt, EndsAt: phase.EndsAt,
	}

	for i, sel := range in.Selections {
		if sel.CategoryID == nil {
			return fmt.Errorf("%w: selection %d missing category", ErrInvalidSelection, i)
		}
		price, err := repo.GetPrice(ctx, phase.ID, *sel.CategoryID, sel.DurationID)
		if err != nil {
			return fmt.Errorf("%w: selection %d (no price for phase %s × category %s × duration %v)",
				ErrInvalidSelection, i, phase.ID, *sel.CategoryID, sel.DurationID)
		}
		li := LineItem{
			Description: phase.Name,
			AmountMinor: AmountMinor(price.AmountMinor),
			PhaseID:     &phase.ID,
			CategoryID:  sel.CategoryID,
			DurationID:  sel.DurationID,
		}
		q.LineItems = append(q.LineItems, li)
		q.SubtotalMinor += li.AmountMinor
	}
	return nil
}

func priceDonation(ctx context.Context, repo Repo, in QuoteInput, q *Quote) error {
	cfg, err := repo.GetDonationConfig(ctx, in.Event.ID)
	if err != nil {
		if errors.Is(err, ErrDonationConfigMissing) {
			return ErrDonationConfigMissing
		}
		return fmt.Errorf("get donation config: %w", err)
	}
	for i, sel := range in.Selections {
		amt := cfg.SuggestedMinor
		if sel.DonationAmountMinor != nil {
			amt = *sel.DonationAmountMinor
		}
		if amt < cfg.MinMinor {
			return fmt.Errorf("%w: selection %d (got %d, min %d)", ErrDonationBelowMin, i, amt, cfg.MinMinor)
		}
		li := LineItem{
			Description: "Donation",
			AmountMinor: AmountMinor(amt),
		}
		q.LineItems = append(q.LineItems, li)
		q.SubtotalMinor += li.AmountMinor
	}
	return nil
}
