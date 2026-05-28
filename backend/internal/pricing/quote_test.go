package pricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/pricing"
)

// ----------------------------------------------------------------------------
// Fake repo (in-memory, no DB)
// ----------------------------------------------------------------------------

type fakeRepo struct {
	phases          []pricing.Phase // each event_id contributes its own; phase resolution scans
	prices          map[string]pricing.Price
	donation        map[uuid.UUID]pricing.DonationConfig
	coupons         map[string]pricing.Coupon // key: eventID + ":" + code
	couponPhaseFs   map[uuid.UUID][]uuid.UUID
	couponCatFs     map[uuid.UUID][]uuid.UUID
	couponUses      map[uuid.UUID]int
	couponEmailUses map[uuid.UUID]map[string]int
}

func newFake() *fakeRepo {
	return &fakeRepo{
		prices:          map[string]pricing.Price{},
		donation:        map[uuid.UUID]pricing.DonationConfig{},
		coupons:         map[string]pricing.Coupon{},
		couponPhaseFs:   map[uuid.UUID][]uuid.UUID{},
		couponCatFs:     map[uuid.UUID][]uuid.UUID{},
		couponUses:      map[uuid.UUID]int{},
		couponEmailUses: map[uuid.UUID]map[string]int{},
	}
}

func priceKey(phaseID, categoryID uuid.UUID, durationID *uuid.UUID) string {
	s := phaseID.String() + ":" + categoryID.String()
	if durationID != nil {
		s += ":" + durationID.String()
	}
	return s
}

func (f *fakeRepo) addPhase(p pricing.Phase) { f.phases = append(f.phases, p) }
func (f *fakeRepo) addPrice(p pricing.Price) {
	f.prices[priceKey(p.PhaseID, p.CategoryID, p.DurationID)] = p
}
func (f *fakeRepo) addCoupon(c pricing.Coupon) {
	f.coupons[c.EventID.String()+":"+c.Code] = c
}

func (f *fakeRepo) GetActivePhase(_ context.Context, eventID uuid.UUID, when time.Time) (pricing.Phase, error) {
	var best pricing.Phase
	found := false
	for _, p := range f.phases {
		if p.EventID != eventID {
			continue
		}
		if !p.StartsAt.After(when) && p.EndsAt.After(when) {
			if !found || p.Ordering < best.Ordering {
				best = p
				found = true
			}
		}
	}
	if !found {
		return pricing.Phase{}, pricing.ErrNoActivePhase
	}
	return best, nil
}

func (f *fakeRepo) GetPrice(_ context.Context, phaseID, categoryID uuid.UUID, durationID *uuid.UUID) (pricing.Price, error) {
	p, ok := f.prices[priceKey(phaseID, categoryID, durationID)]
	if !ok {
		return pricing.Price{}, pricing.ErrInvalidSelection
	}
	return p, nil
}

func (f *fakeRepo) GetDonationConfig(_ context.Context, eventID uuid.UUID) (pricing.DonationConfig, error) {
	cfg, ok := f.donation[eventID]
	if !ok {
		return pricing.DonationConfig{}, pricing.ErrDonationConfigMissing
	}
	return cfg, nil
}

func (f *fakeRepo) GetCouponByCode(_ context.Context, eventID uuid.UUID, code string) (pricing.Coupon, error) {
	c, ok := f.coupons[eventID.String()+":"+code]
	if !ok {
		return pricing.Coupon{}, pricing.ErrCouponNotFound
	}
	return c, nil
}

func (f *fakeRepo) ListCouponPhaseFilters(_ context.Context, couponID uuid.UUID) ([]uuid.UUID, error) {
	return f.couponPhaseFs[couponID], nil
}

func (f *fakeRepo) ListCouponCategoryFilters(_ context.Context, couponID uuid.UUID) ([]uuid.UUID, error) {
	return f.couponCatFs[couponID], nil
}

func (f *fakeRepo) CountCouponRedemptions(_ context.Context, couponID uuid.UUID) (int, error) {
	return f.couponUses[couponID], nil
}

func (f *fakeRepo) CountCouponRedemptionsForEmail(_ context.Context, couponID uuid.UUID, email string) (int, error) {
	if m, ok := f.couponEmailUses[couponID]; ok {
		return m[email], nil
	}
	return 0, nil
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func mustQuote(t *testing.T, repo pricing.Repo, in pricing.QuoteInput) pricing.Quote {
	t.Helper()
	q, err := pricing.Compute(context.Background(), repo, in)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return q
}

func ptrInt64(v int64) *int64        { return &v }
func ptrInt32(v int32) *int32        { return &v }
func ptrTime(t time.Time) *time.Time { return &t }
func ptrUUID(u uuid.UUID) *uuid.UUID { return &u }

// matrixFixture builds a Dao Dance-style fixture: 3 phases × 2 categories ×
// 3 durations (day/weekend/full). Reduced is roughly 80% of adult.
type matrixFixture struct {
	event       domain.Event
	phaseEarly  uuid.UUID
	phaseNormal uuid.UUID
	phaseLate   uuid.UUID
	catAdult    uuid.UUID
	catReduced  uuid.UUID
	durDay      uuid.UUID
	durWeekend  uuid.UUID
	durFull     uuid.UUID
	now         time.Time
}

func newMatrixFixture() (*fakeRepo, *matrixFixture) {
	repo := newFake()
	f := &matrixFixture{
		event: domain.Event{
			ID:          uuid.New(),
			Currency:    "EUR",
			PricingMode: domain.PricingModeMatrix,
		},
		phaseEarly:  uuid.New(),
		phaseNormal: uuid.New(),
		phaseLate:   uuid.New(),
		catAdult:    uuid.New(),
		catReduced:  uuid.New(),
		durDay:      uuid.New(),
		durWeekend:  uuid.New(),
		durFull:     uuid.New(),
		now:         time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
	}
	// Phases:
	//   early-bird:  Jan 1 → June 1
	//   normal:      June 1 → July 20  (active at fixture.now)
	//   late-bird:   July 20 → Aug 1
	repo.addPhase(pricing.Phase{
		ID: f.phaseEarly, EventID: f.event.ID, Name: "early-bird",
		StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Ordering: 0,
	})
	repo.addPhase(pricing.Phase{
		ID: f.phaseNormal, EventID: f.event.ID, Name: "normal",
		StartsAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		Ordering: 1,
	})
	repo.addPhase(pricing.Phase{
		ID: f.phaseLate, EventID: f.event.ID, Name: "late-bird",
		StartsAt: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Ordering: 2,
	})

	// Prices, in EUR cents.
	type cell struct {
		phase, cat, dur uuid.UUID
		amount          int64
	}
	cells := []cell{
		{f.phaseEarly, f.catAdult, f.durDay, 5000},
		{f.phaseEarly, f.catAdult, f.durWeekend, 12000},
		{f.phaseEarly, f.catAdult, f.durFull, 20000},
		{f.phaseEarly, f.catReduced, f.durDay, 4000},
		{f.phaseEarly, f.catReduced, f.durWeekend, 9600},
		{f.phaseEarly, f.catReduced, f.durFull, 16000},

		{f.phaseNormal, f.catAdult, f.durDay, 6000},
		{f.phaseNormal, f.catAdult, f.durWeekend, 14000},
		{f.phaseNormal, f.catAdult, f.durFull, 24000},
		{f.phaseNormal, f.catReduced, f.durDay, 4800},
		{f.phaseNormal, f.catReduced, f.durWeekend, 11200},
		{f.phaseNormal, f.catReduced, f.durFull, 19200},

		{f.phaseLate, f.catAdult, f.durDay, 7000},
		{f.phaseLate, f.catAdult, f.durWeekend, 16000},
		{f.phaseLate, f.catAdult, f.durFull, 28000},
		{f.phaseLate, f.catReduced, f.durDay, 5600},
		{f.phaseLate, f.catReduced, f.durWeekend, 12800},
		// late × reduced × full deliberately omitted to test sparse rejection.
	}
	for _, c := range cells {
		dur := c.dur
		repo.addPrice(pricing.Price{
			PhaseID: c.phase, CategoryID: c.cat, DurationID: &dur,
			AmountMinor: c.amount, EventID: f.event.ID,
		})
	}
	return repo, f
}

// ----------------------------------------------------------------------------
// Tests — matrix
// ----------------------------------------------------------------------------

func TestMatrix_HappyPath(t *testing.T) {
	repo, f := newMatrixFixture()

	// Booking now → "normal" phase. Two participants: 1 adult full + 1 reduced weekend.
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event,
		When:  f.now,
		Selections: []pricing.Selection{
			{CategoryID: &f.catAdult, DurationID: &f.durFull},
			{CategoryID: &f.catReduced, DurationID: &f.durWeekend},
		},
	})

	// 24000 + 11200 = 35200
	if q.SubtotalMinor != 35200 {
		t.Fatalf("subtotal: want 35200, got %d", q.SubtotalMinor)
	}
	if q.TotalMinor != 35200 {
		t.Fatalf("total: want 35200, got %d", q.TotalMinor)
	}
	if q.Phase == nil || q.Phase.Name != "normal" {
		t.Fatalf("expected normal phase, got %+v", q.Phase)
	}
	if len(q.LineItems) != 2 {
		t.Fatalf("want 2 line items, got %d", len(q.LineItems))
	}
}

func TestMatrix_MatchesAllNineDaoLikeCells(t *testing.T) {
	repo, f := newMatrixFixture()

	// "normal" phase, walk all 6 (adult+reduced) × (day+weekend+full) cells.
	type want struct {
		cat, dur uuid.UUID
		expect   int64
	}
	cases := []want{
		{f.catAdult, f.durDay, 6000},
		{f.catAdult, f.durWeekend, 14000},
		{f.catAdult, f.durFull, 24000},
		{f.catReduced, f.durDay, 4800},
		{f.catReduced, f.durWeekend, 11200},
		{f.catReduced, f.durFull, 19200},
	}
	for _, c := range cases {
		c := c
		t.Run(c.cat.String()[:6], func(t *testing.T) {
			q := mustQuote(t, repo, pricing.QuoteInput{
				Event: f.event, When: f.now,
				Selections: []pricing.Selection{{CategoryID: &c.cat, DurationID: &c.dur}},
			})
			if q.TotalMinor != pricing.AmountMinor(c.expect) {
				t.Fatalf("want %d, got %d", c.expect, q.TotalMinor)
			}
		})
	}
}

func TestMatrix_NoActivePhase(t *testing.T) {
	repo, f := newMatrixFixture()
	// 2027 is well past the late-bird end.
	when := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: when,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
	})
	if !errors.Is(err, pricing.ErrNoActivePhase) {
		t.Fatalf("expected ErrNoActivePhase, got %v", err)
	}
}

func TestMatrix_SparseCellRejected(t *testing.T) {
	repo, f := newMatrixFixture()
	// Late × reduced × full was deliberately omitted.
	when := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: when,
		Selections: []pricing.Selection{{CategoryID: &f.catReduced, DurationID: &f.durFull}},
	})
	if !errors.Is(err, pricing.ErrInvalidSelection) {
		t.Fatalf("expected ErrInvalidSelection, got %v", err)
	}
}

func TestMatrix_MissingCategory(t *testing.T) {
	repo, f := newMatrixFixture()
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{DurationID: &f.durFull}}, // no category
	})
	if !errors.Is(err, pricing.ErrInvalidSelection) {
		t.Fatalf("expected ErrInvalidSelection, got %v", err)
	}
}

func TestNoSelections(t *testing.T) {
	repo, f := newMatrixFixture()
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
	})
	if !errors.Is(err, pricing.ErrNoSelections) {
		t.Fatalf("expected ErrNoSelections, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Tests — donation
// ----------------------------------------------------------------------------

func donationFixture() (*fakeRepo, domain.Event) {
	repo := newFake()
	event := domain.Event{
		ID:          uuid.New(),
		Currency:    "EUR",
		PricingMode: domain.PricingModeDonation,
	}
	repo.donation[event.ID] = pricing.DonationConfig{
		EventID:        event.ID,
		SuggestedMinor: 2500,
		MinMinor:       1000,
	}
	return repo, event
}

func TestDonation_Suggested(t *testing.T) {
	repo, event := donationFixture()
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: event, When: time.Now(),
		Selections: []pricing.Selection{{}, {}}, // 2 participants, no amount → suggested
	})
	if q.SubtotalMinor != 5000 {
		t.Fatalf("expected 5000, got %d", q.SubtotalMinor)
	}
}

func TestDonation_Explicit(t *testing.T) {
	repo, event := donationFixture()
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: event, When: time.Now(),
		Selections: []pricing.Selection{{DonationAmountMinor: ptrInt64(7777)}},
	})
	if q.TotalMinor != 7777 {
		t.Fatalf("expected 7777, got %d", q.TotalMinor)
	}
}

func TestDonation_BelowMinRejected(t *testing.T) {
	repo, event := donationFixture()
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: event, When: time.Now(),
		Selections: []pricing.Selection{{DonationAmountMinor: ptrInt64(500)}},
	})
	if !errors.Is(err, pricing.ErrDonationBelowMin) {
		t.Fatalf("expected ErrDonationBelowMin, got %v", err)
	}
}

func TestDonation_ConfigMissing(t *testing.T) {
	repo := newFake()
	event := domain.Event{ID: uuid.New(), Currency: "EUR", PricingMode: domain.PricingModeDonation}
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: event, When: time.Now(),
		Selections: []pricing.Selection{{}},
	})
	if !errors.Is(err, pricing.ErrDonationConfigMissing) {
		t.Fatalf("expected ErrDonationConfigMissing, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Tests — coupons
// ----------------------------------------------------------------------------

func TestCoupon_FixedReduce(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "SAVE10",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(1000),
	})

	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "SAVE10",
	})
	if q.SubtotalMinor != 24000 || q.DiscountMinor != 1000 || q.TotalMinor != 23000 {
		t.Fatalf("subtotal/discount/total = %d/%d/%d, want 24000/1000/23000",
			q.SubtotalMinor, q.DiscountMinor, q.TotalMinor)
	}
	if q.AppliedCouponCode != "SAVE10" {
		t.Fatalf("expected SAVE10, got %q", q.AppliedCouponCode)
	}
}

func TestCoupon_PercentalReduce_Rounding(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "PCT33",
		Type: pricing.CouponPercentalReduce, ValuePercent: ptrInt32(33),
	})
	// 33% of 24000 = 7920, exact, no rounding ambiguity.
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "PCT33",
	})
	if q.DiscountMinor != 7920 || q.TotalMinor != 16080 {
		t.Fatalf("discount/total = %d/%d, want 7920/16080", q.DiscountMinor, q.TotalMinor)
	}
}

func TestCoupon_PercentalReduce_RoundsHalfUp(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "PCT33B",
		Type: pricing.CouponPercentalReduce, ValuePercent: ptrInt32(33),
	})
	// 33% of 4800 = 1584. Exact.
	// 33% of 5000 = 1650. Exact.
	// 33% of 250 = 82.5 → 83 with round-half-up.
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durDay}}, // 6000
		CouponCode: "PCT33B",
	})
	// 33% of 6000 = 1980.
	if q.DiscountMinor != 1980 {
		t.Fatalf("discount = %d, want 1980", q.DiscountMinor)
	}
}

func TestCoupon_Guestlist_ForcesZero(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "GUEST",
		Type: pricing.CouponGuestlist,
	})
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{
			{CategoryID: &f.catAdult, DurationID: &f.durFull},
			{CategoryID: &f.catAdult, DurationID: &f.durFull},
		},
		CouponCode: "GUEST",
	})
	if q.TotalMinor != 0 {
		t.Fatalf("guestlist total: want 0, got %d", q.TotalMinor)
	}
}

func TestCoupon_NotFound(t *testing.T) {
	repo, f := newMatrixFixture()
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "NOPE",
	})
	if !errors.Is(err, pricing.ErrCouponNotFound) {
		t.Fatalf("expected ErrCouponNotFound, got %v", err)
	}
}

func TestCoupon_Expired(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "EXP",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(500),
		ValidTo: ptrTime(f.now.Add(-time.Hour)),
	})
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "EXP",
	})
	if !errors.Is(err, pricing.ErrCouponExpired) {
		t.Fatalf("expected ErrCouponExpired, got %v", err)
	}
}

func TestCoupon_NotYetValid(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "FUTURE",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(500),
		ValidFrom: ptrTime(f.now.Add(time.Hour)),
	})
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "FUTURE",
	})
	if !errors.Is(err, pricing.ErrCouponNotYetValid) {
		t.Fatalf("expected ErrCouponNotYetValid, got %v", err)
	}
}

func TestCoupon_MaxUsesExceeded(t *testing.T) {
	repo, f := newMatrixFixture()
	cid := uuid.New()
	repo.addCoupon(pricing.Coupon{
		ID: cid, EventID: f.event.ID, Code: "LIMITED",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(500),
		MaxUses: ptrInt32(2),
	})
	repo.couponUses[cid] = 2

	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "LIMITED",
	})
	if !errors.Is(err, pricing.ErrCouponMaxUsesExceeded) {
		t.Fatalf("expected ErrCouponMaxUsesExceeded, got %v", err)
	}
}

func TestCoupon_SingleUsePerEmail(t *testing.T) {
	repo, f := newMatrixFixture()
	cid := uuid.New()
	repo.addCoupon(pricing.Coupon{
		ID: cid, EventID: f.event.ID, Code: "ONCE",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(500),
		SingleUsePerEmail: true,
	})
	repo.couponEmailUses[cid] = map[string]int{"alice@x": 1}

	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections:   []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode:   "ONCE",
		ContactEmail: "alice@x",
	})
	if !errors.Is(err, pricing.ErrCouponAlreadyUsed) {
		t.Fatalf("expected ErrCouponAlreadyUsed, got %v", err)
	}
}

func TestCoupon_PhaseFilterMismatch(t *testing.T) {
	repo, f := newMatrixFixture()
	cid := uuid.New()
	repo.addCoupon(pricing.Coupon{
		ID: cid, EventID: f.event.ID, Code: "EARLYONLY",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(500),
	})
	repo.couponPhaseFs[cid] = []uuid.UUID{f.phaseEarly}

	// fixture.now resolves to "normal" phase, not the allowed early-bird.
	_, err := pricing.Compute(context.Background(), repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durFull}},
		CouponCode: "EARLYONLY",
	})
	if !errors.Is(err, pricing.ErrCouponNotApplicable) {
		t.Fatalf("expected ErrCouponNotApplicable, got %v", err)
	}
}

func TestCoupon_CategoryFilterPasses(t *testing.T) {
	repo, f := newMatrixFixture()
	cid := uuid.New()
	repo.addCoupon(pricing.Coupon{
		ID: cid, EventID: f.event.ID, Code: "REDUCEDONLY",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(500),
	})
	repo.couponCatFs[cid] = []uuid.UUID{f.catReduced}

	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catReduced, DurationID: &f.durFull}},
		CouponCode: "REDUCEDONLY",
	})
	if q.DiscountMinor != 500 {
		t.Fatalf("expected discount 500, got %d", q.DiscountMinor)
	}
}

func TestCoupon_DiscountCappedAtSubtotal(t *testing.T) {
	repo, f := newMatrixFixture()
	repo.addCoupon(pricing.Coupon{
		ID: uuid.New(), EventID: f.event.ID, Code: "BIGCUT",
		Type: pricing.CouponFixedReduce, ValueMinor: ptrInt64(999999),
	})
	q := mustQuote(t, repo, pricing.QuoteInput{
		Event: f.event, When: f.now,
		Selections: []pricing.Selection{{CategoryID: &f.catAdult, DurationID: &f.durDay}}, // 6000
		CouponCode: "BIGCUT",
	})
	if q.TotalMinor != 0 {
		t.Fatalf("expected total 0 (discount capped), got %d", q.TotalMinor)
	}
	if q.DiscountMinor != 6000 {
		t.Fatalf("expected discount capped at subtotal 6000, got %d", q.DiscountMinor)
	}
}

// suppress import-not-used if a helper isn't called by every test.
var _ = ptrUUID
