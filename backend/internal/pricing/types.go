// Package pricing implements the pure-logic Quote engine and the related
// pricing-config admin handlers. The engine is the only path that produces
// booking totals — see Quote().
package pricing

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// AmountMinor is money in the currency's minor unit (cents for EUR).
type AmountMinor int64

// Selection is the per-participant choice the booker makes.
//
// In matrix mode CategoryID is required; DurationID is set when the event
// has a duration dimension, nil otherwise.
//
// In donation mode DonationAmountMinor is the per-participant amount.
// nil means "use the suggested amount from donation_configs".
type Selection struct {
	CategoryID          *uuid.UUID
	DurationID          *uuid.UUID
	DonationAmountMinor *int64
}

// LineItem is one row of a Quote. Snapshot of the selection lives here so
// it can be copied onto the corresponding ticket at booking time and survive
// later changes to the pricing config.
type LineItem struct {
	Description string
	AmountMinor AmountMinor
	PhaseID     *uuid.UUID
	CategoryID  *uuid.UUID
	DurationID  *uuid.UUID
}

// Quote is the result of pricing a set of selections.
//
// SubtotalMinor is the sum of LineItems before any coupon. DiscountMinor is
// the coupon-applied reduction (capped at SubtotalMinor; never negative).
// TotalMinor = SubtotalMinor - DiscountMinor.
type Quote struct {
	Currency          string
	LineItems         []LineItem
	SubtotalMinor     AmountMinor
	DiscountMinor     AmountMinor
	TotalMinor        AmountMinor
	AppliedCouponID   *uuid.UUID
	AppliedCouponCode string
	Phase             *PhaseSnapshot // populated for matrix mode; nil for donation
}

// PhaseSnapshot is the active phase resolved at quote time. Stored on the
// Quote so callers (and tests) can see which phase priced each line item.
type PhaseSnapshot struct {
	ID       uuid.UUID
	Name     string
	StartsAt time.Time
	EndsAt   time.Time
}

// Errors returned by Quote().
var (
	ErrNoSelections          = errors.New("pricing: no selections")
	ErrNoActivePhase         = errors.New("pricing: no active price phase")
	ErrInvalidSelection      = errors.New("pricing: invalid selection")
	ErrDonationBelowMin      = errors.New("pricing: donation below minimum")
	ErrDonationConfigMissing = errors.New("pricing: donation config not configured")

	ErrCouponNotFound        = errors.New("pricing: coupon not found")
	ErrCouponNotYetValid     = errors.New("pricing: coupon not yet valid")
	ErrCouponExpired         = errors.New("pricing: coupon expired")
	ErrCouponMaxUsesExceeded = errors.New("pricing: coupon max uses exceeded")
	ErrCouponAlreadyUsed     = errors.New("pricing: coupon already used by this email")
	ErrCouponNotApplicable   = errors.New("pricing: coupon not applicable to this selection")
)
