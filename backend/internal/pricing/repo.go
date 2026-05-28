package pricing

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Phase, Price, DonationConfig, Coupon are the minimal shapes the engine
// needs from persistence. They mirror the sqlc rows but stay decoupled so
// the engine can be unit-tested with a fake repo.

type Phase struct {
	ID       uuid.UUID
	EventID  uuid.UUID
	Name     string
	StartsAt time.Time
	EndsAt   time.Time
	Ordering int32
}

type Price struct {
	ID          uuid.UUID
	EventID     uuid.UUID
	PhaseID     uuid.UUID
	CategoryID  uuid.UUID
	DurationID  *uuid.UUID
	AmountMinor int64
}

type DonationConfig struct {
	EventID        uuid.UUID
	SuggestedMinor int64
	MinMinor       int64
}

type CouponType string

const (
	CouponFixedReduce     CouponType = "fixed_reduce"
	CouponPercentalReduce CouponType = "percental_reduce"
	CouponGuestlist       CouponType = "guestlist"
)

type Coupon struct {
	ID                uuid.UUID
	EventID           uuid.UUID
	Code              string
	Type              CouponType
	ValueMinor        *int64
	ValuePercent      *int32
	MaxUses           *int32
	ValidFrom         *time.Time
	ValidTo           *time.Time
	SingleUsePerEmail bool
}

// Repo is what Quote() and RedeemCoupon() need from persistence. Implemented
// by Store (backed by sqlc) for production and by fakes in tests.
type Repo interface {
	GetActivePhase(ctx context.Context, eventID uuid.UUID, when time.Time) (Phase, error)
	GetPrice(ctx context.Context, phaseID, categoryID uuid.UUID, durationID *uuid.UUID) (Price, error)
	GetDonationConfig(ctx context.Context, eventID uuid.UUID) (DonationConfig, error)

	GetCouponByCode(ctx context.Context, eventID uuid.UUID, code string) (Coupon, error)
	ListCouponPhaseFilters(ctx context.Context, couponID uuid.UUID) ([]uuid.UUID, error)
	ListCouponCategoryFilters(ctx context.Context, couponID uuid.UUID) ([]uuid.UUID, error)
	CountCouponRedemptions(ctx context.Context, couponID uuid.UUID) (int, error)
	CountCouponRedemptionsForEmail(ctx context.Context, couponID uuid.UUID, email string) (int, error)
}
