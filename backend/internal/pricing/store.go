package pricing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// Store is the production Repo implementation, backed by sqlc-generated queries.
type Store struct {
	q *db.Queries
}

func NewStore(q *db.Queries) *Store {
	return &Store{q: q}
}

func (s *Store) GetActivePhase(ctx context.Context, eventID uuid.UUID, when time.Time) (Phase, error) {
	row, err := s.q.GetActivePhaseForEvent(ctx, db.GetActivePhaseForEventParams{
		EventID:  eventID,
		StartsAt: when,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Phase{}, ErrNoActivePhase
		}
		return Phase{}, err
	}
	return Phase{
		ID:       row.ID,
		EventID:  row.EventID,
		Name:     row.Name,
		StartsAt: row.StartsAt,
		EndsAt:   row.EndsAt,
		Ordering: row.Ordering,
	}, nil
}

func (s *Store) GetPrice(ctx context.Context, phaseID, categoryID uuid.UUID, durationID *uuid.UUID) (Price, error) {
	row, err := s.q.GetPriceFor(ctx, db.GetPriceForParams{
		PhaseID:    phaseID,
		CategoryID: categoryID,
		DurationID: durationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Price{}, ErrInvalidSelection
		}
		return Price{}, err
	}
	return Price{
		ID:          row.ID,
		EventID:     row.EventID,
		PhaseID:     row.PhaseID,
		CategoryID:  row.CategoryID,
		DurationID:  row.DurationID,
		AmountMinor: row.AmountMinor,
	}, nil
}

func (s *Store) GetDonationConfig(ctx context.Context, eventID uuid.UUID) (DonationConfig, error) {
	row, err := s.q.GetDonationConfig(ctx, eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DonationConfig{}, ErrDonationConfigMissing
		}
		return DonationConfig{}, err
	}
	return DonationConfig{
		EventID:        row.EventID,
		SuggestedMinor: row.SuggestedMinor,
		MinMinor:       row.MinMinor,
	}, nil
}

func (s *Store) GetCouponByCode(ctx context.Context, eventID uuid.UUID, code string) (Coupon, error) {
	row, err := s.q.GetCouponByCode(ctx, db.GetCouponByCodeParams{EventID: eventID, Code: code})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Coupon{}, ErrCouponNotFound
		}
		return Coupon{}, err
	}
	return mapCoupon(row), nil
}

func (s *Store) ListCouponPhaseFilters(ctx context.Context, couponID uuid.UUID) ([]uuid.UUID, error) {
	return s.q.ListCouponPhaseFilters(ctx, couponID)
}

func (s *Store) ListCouponCategoryFilters(ctx context.Context, couponID uuid.UUID) ([]uuid.UUID, error) {
	return s.q.ListCouponCategoryFilters(ctx, couponID)
}

func (s *Store) CountCouponRedemptions(ctx context.Context, couponID uuid.UUID) (int, error) {
	v, err := s.q.CountCouponRedemptions(ctx, couponID)
	return int(v), err
}

func (s *Store) CountCouponRedemptionsForEmail(ctx context.Context, couponID uuid.UUID, email string) (int, error) {
	v, err := s.q.CountCouponRedemptionsForEmail(ctx, db.CountCouponRedemptionsForEmailParams{
		CouponID: couponID,
		Lower:    email,
	})
	return int(v), err
}

func mapCoupon(row db.Coupon) Coupon {
	return Coupon{
		ID:                row.ID,
		EventID:           row.EventID,
		Code:              row.Code,
		Type:              CouponType(row.Type),
		ValueMinor:        row.ValueMinor,
		ValuePercent:      row.ValuePercent,
		MaxUses:           row.MaxUses,
		ValidFrom:         row.ValidFrom,
		ValidTo:           row.ValidTo,
		SingleUsePerEmail: row.SingleUsePerEmail,
	}
}
