package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// ============================================================================
// Snapshot read (one big GET)
// ============================================================================

type pricingSnapshot struct {
	PricingMode    string          `json:"pricing_mode"`
	Currency       string          `json:"currency"`
	DonationConfig *donationCfgDTO `json:"donation_config,omitempty"`
	Phases         []phaseDTO      `json:"phases"`
	Categories     []categoryDTO   `json:"categories"`
	Durations      []durationDTO   `json:"durations"`
	Prices         []priceDTO      `json:"prices"`
	Coupons        []couponDTO     `json:"coupons"`
}

type donationCfgDTO struct {
	SuggestedMinor int64 `json:"suggested_minor"`
	MinMinor       int64 `json:"min_minor"`
}

type phaseDTO struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Ordering int       `json:"ordering"`
}

type categoryDTO struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Ordering int       `json:"ordering"`
}

type durationDTO struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Ordering int       `json:"ordering"`
}

type priceDTO struct {
	ID          uuid.UUID  `json:"id"`
	PhaseID     uuid.UUID  `json:"phase_id"`
	CategoryID  uuid.UUID  `json:"category_id"`
	DurationID  *uuid.UUID `json:"duration_id"`
	AmountMinor int64      `json:"amount_minor"`
}

type couponDTO struct {
	ID                uuid.UUID  `json:"id"`
	Code              string     `json:"code"`
	Type              string     `json:"type"`
	ValueMinor        *int64     `json:"value_minor"`
	ValuePercent      *int32     `json:"value_percent"`
	MaxUses           *int32     `json:"max_uses"`
	ValidFrom         *time.Time `json:"valid_from"`
	ValidTo           *time.Time `json:"valid_to"`
	SingleUsePerEmail bool       `json:"single_use_per_email"`
}

// HandlePricingSnapshot: GET /api/admin/events/{id}/pricing
func (s *Service) HandlePricingSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	q := s.pool.Queries()

	event, err := q.GetEventByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	out := pricingSnapshot{
		PricingMode: event.PricingMode,
		Currency:    event.Currency,
		Phases:      []phaseDTO{},
		Categories:  []categoryDTO{},
		Durations:   []durationDTO{},
		Prices:      []priceDTO{},
		Coupons:     []couponDTO{},
	}

	if d, err := q.GetDonationConfig(r.Context(), id); err == nil {
		out.DonationConfig = &donationCfgDTO{SuggestedMinor: d.SuggestedMinor, MinMinor: d.MinMinor}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	phases, _ := q.ListPricePhases(r.Context(), id)
	for _, p := range phases {
		out.Phases = append(out.Phases, phaseDTO{
			ID: p.ID, Name: p.Name, StartsAt: p.StartsAt, EndsAt: p.EndsAt, Ordering: int(p.Ordering),
		})
	}
	cats, _ := q.ListPriceCategories(r.Context(), id)
	for _, c := range cats {
		out.Categories = append(out.Categories, categoryDTO{ID: c.ID, Name: c.Name, Ordering: int(c.Ordering)})
	}
	durs, _ := q.ListPriceDurations(r.Context(), id)
	for _, d := range durs {
		out.Durations = append(out.Durations, durationDTO{ID: d.ID, Name: d.Name, Ordering: int(d.Ordering)})
	}
	prices, _ := q.ListPrices(r.Context(), id)
	for _, p := range prices {
		out.Prices = append(out.Prices, priceDTO{
			ID: p.ID, PhaseID: p.PhaseID, CategoryID: p.CategoryID,
			DurationID: p.DurationID, AmountMinor: p.AmountMinor,
		})
	}
	coupons, _ := q.ListCoupons(r.Context(), id)
	for _, c := range coupons {
		out.Coupons = append(out.Coupons, couponDTO{
			ID: c.ID, Code: c.Code, Type: c.Type,
			ValueMinor: c.ValueMinor, ValuePercent: c.ValuePercent,
			MaxUses: c.MaxUses, ValidFrom: c.ValidFrom, ValidTo: c.ValidTo,
			SingleUsePerEmail: c.SingleUsePerEmail,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

// ============================================================================
// Donation config
// ============================================================================

type donationReq struct {
	SuggestedMinor int64 `json:"suggested_minor"`
	MinMinor       int64 `json:"min_minor"`
}

// HandleUpsertDonationConfig: PUT /api/admin/events/{id}/pricing/donation
func (s *Service) HandleUpsertDonationConfig(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())

	var req donationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.SuggestedMinor < 0 || req.MinMinor < 0 {
		writeErr(w, http.StatusBadRequest, "invalid_amount", "amounts must be non-negative")
		return
	}
	if req.SuggestedMinor < req.MinMinor {
		writeErr(w, http.StatusBadRequest, "invalid_amount", "suggested must be >= min")
		return
	}

	row, err := s.pool.Queries().UpsertDonationConfig(r.Context(), db.UpsertDonationConfigParams{
		EventID: id, SuggestedMinor: req.SuggestedMinor, MinMinor: req.MinMinor,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionPricingUpdate, map[string]any{"section": "donation"})
	writeJSON(w, http.StatusOK, donationCfgDTO{SuggestedMinor: row.SuggestedMinor, MinMinor: row.MinMinor})
}

// ============================================================================
// Phases
// ============================================================================

type phaseReq struct {
	Name     string    `json:"name"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Ordering int       `json:"ordering"`
}

func (p phaseReq) validate() error {
	if p.Name == "" {
		return fmt.Errorf("name_required")
	}
	if p.StartsAt.IsZero() || p.EndsAt.IsZero() || !p.EndsAt.After(p.StartsAt) {
		return fmt.Errorf("invalid_dates")
	}
	return nil
}

// HandleCreatePhase: POST /api/admin/events/{id}/pricing/phases
func (s *Service) HandleCreatePhase(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())

	var req phaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), err.Error())
		return
	}

	row, err := s.pool.Queries().CreatePricePhase(r.Context(), db.CreatePricePhaseParams{
		EventID:  id,
		Name:     req.Name,
		StartsAt: req.StartsAt,
		EndsAt:   req.EndsAt,
		Ordering: int32(req.Ordering),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionPricingUpdate, map[string]any{
		"section": "phase_create", "phase_id": row.ID.String(),
	})
	writeJSON(w, http.StatusCreated, phaseDTO{ID: row.ID, Name: row.Name, StartsAt: row.StartsAt, EndsAt: row.EndsAt, Ordering: int(row.Ordering)})
}

// HandleDeletePhase: DELETE /api/admin/events/{id}/pricing/phases/{phaseId}
func (s *Service) HandleDeletePhase(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	phaseID, err := uuid.Parse(chi.URLParam(r, "phaseId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_phase_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	if err := s.pool.Queries().DeletePricePhase(r.Context(), phaseID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionPricingUpdate, map[string]any{
		"section": "phase_delete", "phase_id": phaseID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Categories
// ============================================================================

type namedReq struct {
	Name     string `json:"name"`
	Ordering int    `json:"ordering"`
}

// HandleCreateCategory: POST /api/admin/events/{id}/pricing/categories
func (s *Service) HandleCreateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	var req namedReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name required")
		return
	}
	row, err := s.pool.Queries().CreatePriceCategory(r.Context(), db.CreatePriceCategoryParams{
		EventID: id, Name: req.Name, Ordering: int32(req.Ordering),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionPricingUpdate, map[string]any{
		"section": "category_create", "category_id": row.ID.String(),
	})
	writeJSON(w, http.StatusCreated, categoryDTO{ID: row.ID, Name: row.Name, Ordering: int(row.Ordering)})
}

// HandleDeleteCategory: DELETE /api/admin/events/{id}/pricing/categories/{catId}
func (s *Service) HandleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	catID, err := uuid.Parse(chi.URLParam(r, "catId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_category_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	if err := s.pool.Queries().DeletePriceCategory(r.Context(), catID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionPricingUpdate, map[string]any{
		"section": "category_delete", "category_id": catID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Durations
// ============================================================================

// HandleCreateDuration: POST /api/admin/events/{id}/pricing/durations
func (s *Service) HandleCreateDuration(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	var req namedReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name required")
		return
	}
	row, err := s.pool.Queries().CreatePriceDuration(r.Context(), db.CreatePriceDurationParams{
		EventID: id, Name: req.Name, Ordering: int32(req.Ordering),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionPricingUpdate, map[string]any{
		"section": "duration_create", "duration_id": row.ID.String(),
	})
	writeJSON(w, http.StatusCreated, durationDTO{ID: row.ID, Name: row.Name, Ordering: int(row.Ordering)})
}

// HandleDeleteDuration: DELETE /api/admin/events/{id}/pricing/durations/{durId}
func (s *Service) HandleDeleteDuration(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	durID, err := uuid.Parse(chi.URLParam(r, "durId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_duration_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	if err := s.pool.Queries().DeletePriceDuration(r.Context(), durID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionPricingUpdate, map[string]any{
		"section": "duration_delete", "duration_id": durID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Prices (sparse cells)
// ============================================================================

type priceReq struct {
	PhaseID     uuid.UUID  `json:"phase_id"`
	CategoryID  uuid.UUID  `json:"category_id"`
	DurationID  *uuid.UUID `json:"duration_id"`
	AmountMinor int64      `json:"amount_minor"`
}

// HandleUpsertPrice: PUT /api/admin/events/{id}/pricing/prices
func (s *Service) HandleUpsertPrice(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	var req priceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.AmountMinor < 0 {
		writeErr(w, http.StatusBadRequest, "invalid_amount", "must be >= 0")
		return
	}
	if req.PhaseID == uuid.Nil || req.CategoryID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "phase_id and category_id required")
		return
	}
	row, err := s.pool.Queries().UpsertPrice(r.Context(), db.UpsertPriceParams{
		EventID:     id,
		PhaseID:     req.PhaseID,
		CategoryID:  req.CategoryID,
		DurationID:  req.DurationID,
		AmountMinor: req.AmountMinor,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionPricingUpdate, map[string]any{
		"section": "price_upsert", "phase_id": req.PhaseID.String(), "category_id": req.CategoryID.String(),
	})
	writeJSON(w, http.StatusOK, priceDTO{
		ID: row.ID, PhaseID: row.PhaseID, CategoryID: row.CategoryID,
		DurationID: row.DurationID, AmountMinor: row.AmountMinor,
	})
}

// HandleDeletePrice: DELETE /api/admin/events/{id}/pricing/prices/{priceId}
func (s *Service) HandleDeletePrice(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	priceID, err := uuid.Parse(chi.URLParam(r, "priceId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_price_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	if err := s.pool.Queries().DeletePrice(r.Context(), priceID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionPricingUpdate, map[string]any{
		"section": "price_delete", "price_id": priceID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Coupons
// ============================================================================

type couponReq struct {
	Code              string      `json:"code"`
	Type              string      `json:"type"`
	ValueMinor        *int64      `json:"value_minor"`
	ValuePercent      *int32      `json:"value_percent"`
	MaxUses           *int32      `json:"max_uses"`
	ValidFrom         *time.Time  `json:"valid_from"`
	ValidTo           *time.Time  `json:"valid_to"`
	SingleUsePerEmail bool        `json:"single_use_per_email"`
	PhaseFilters      []uuid.UUID `json:"phase_filters"`
	CategoryFilters   []uuid.UUID `json:"category_filters"`
}

func (c couponReq) validate() error {
	if c.Code == "" {
		return fmt.Errorf("code_required")
	}
	switch c.Type {
	case "fixed_reduce":
		if c.ValueMinor == nil || *c.ValueMinor <= 0 {
			return fmt.Errorf("fixed_reduce_requires_positive_value_minor")
		}
		if c.ValuePercent != nil {
			return fmt.Errorf("fixed_reduce_must_not_have_value_percent")
		}
	case "percental_reduce":
		if c.ValuePercent == nil || *c.ValuePercent <= 0 || *c.ValuePercent > 100 {
			return fmt.Errorf("percental_reduce_requires_value_percent_in_1_100")
		}
		if c.ValueMinor != nil {
			return fmt.Errorf("percental_reduce_must_not_have_value_minor")
		}
	case "guestlist":
		if c.ValueMinor != nil || c.ValuePercent != nil {
			return fmt.Errorf("guestlist_must_not_have_values")
		}
	default:
		return fmt.Errorf("invalid_coupon_type")
	}
	return nil
}

// HandleCreateCoupon: POST /api/admin/events/{id}/pricing/coupons
func (s *Service) HandleCreateCoupon(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	var req couponReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), err.Error())
		return
	}

	var created db.Coupon
	err = s.pool.WithTx(r.Context(), func(q *db.Queries) error {
		row, err := q.CreateCoupon(r.Context(), db.CreateCouponParams{
			EventID:           id,
			Code:              req.Code,
			Type:              req.Type,
			ValueMinor:        req.ValueMinor,
			ValuePercent:      req.ValuePercent,
			MaxUses:           req.MaxUses,
			ValidFrom:         req.ValidFrom,
			ValidTo:           req.ValidTo,
			SingleUsePerEmail: req.SingleUsePerEmail,
		})
		if err != nil {
			return err
		}
		created = row
		for _, ph := range req.PhaseFilters {
			if err := q.AddCouponPhaseFilter(r.Context(), db.AddCouponPhaseFilterParams{CouponID: row.ID, PhaseID: ph}); err != nil {
				return err
			}
		}
		for _, cat := range req.CategoryFilters {
			if err := q.AddCouponCategoryFilter(r.Context(), db.AddCouponCategoryFilterParams{CouponID: row.ID, CategoryID: cat}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionCouponCreate, map[string]any{
		"coupon_id": created.ID.String(), "code": created.Code,
	})
	writeJSON(w, http.StatusCreated, couponDTO{
		ID: created.ID, Code: created.Code, Type: created.Type,
		ValueMinor: created.ValueMinor, ValuePercent: created.ValuePercent,
		MaxUses: created.MaxUses, ValidFrom: created.ValidFrom, ValidTo: created.ValidTo,
		SingleUsePerEmail: created.SingleUsePerEmail,
	})
}

// HandleDeleteCoupon: DELETE /api/admin/events/{id}/pricing/coupons/{couponId}
func (s *Service) HandleDeleteCoupon(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	couponID, err := uuid.Parse(chi.URLParam(r, "couponId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_coupon_id", "must be a UUID")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	if err := s.pool.Queries().DeleteCoupon(r.Context(), couponID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionPricingUpdate, map[string]any{
		"section": "coupon_delete", "coupon_id": couponID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}
