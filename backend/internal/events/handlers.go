package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
)

// ============================================================================
// Defaults & helpers
// ============================================================================

// Base palette for events that don't override their colors. Matches the
// base "EP release party" feel.
const (
	defaultColorPrimary   = "#5E576A"
	defaultColorSecondary = "#F5F1EE"
	defaultColorText      = "#1A1A1A"
	defaultCurrency       = "EUR"
	defaultLocale         = "de"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, devMsg string) {
	writeJSON(w, status, map[string]any{"error": code, "developer_message": devMsg})
}

func parseEventID(r *http.Request) (uuid.UUID, error) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func ptrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ============================================================================
// List
// ============================================================================

// HandleList: GET /api/admin/events
//
// Scope by role:
//   - global_admin sees every event;
//   - event_admin / event_manager see only events they're assigned to.
func (s *Service) HandleList(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	q := s.pool.Queries()
	var rows []db.Event
	var err error
	switch user.Role {
	case domain.RoleGlobalAdmin:
		rows, err = q.ListEventsForGlobalAdmin(r.Context())
	default:
		rows, err = q.ListEventsForUser(r.Context(), user.ID)
	}
	if err != nil {
		logging.FromContext(r.Context()).Error("list events failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	out := make([]EventResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, toEventResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}

// ============================================================================
// Get one
// ============================================================================

// HandleGet: GET /api/admin/events/{id}
func (s *Service) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	row, err := s.pool.Queries().GetEventByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEventResponse(row))
}

// ============================================================================
// Create
// ============================================================================

type createRequest struct {
	Slug             string    `json:"slug"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	ColorPrimary     string    `json:"color_primary"`
	ColorSecondary   string    `json:"color_secondary"`
	ColorText        string    `json:"color_text"`
	Location         string    `json:"location"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	ParticipantLimit *int      `json:"participant_limit"`
	PricingMode      string    `json:"pricing_mode"`
	Currency         string    `json:"currency"`
	DefaultLocale    string    `json:"default_locale"`
	IsPublic         bool      `json:"is_public"`

	PaymentTiming     string `json:"payment_timing"`
	BankIBAN          string `json:"bank_iban"`
	BankBIC           string `json:"bank_bic"`
	BankAccountHolder string `json:"bank_account_holder"`
	PaypalHandle      string `json:"paypal_handle"`
	PaymentTestMode   bool   `json:"payment_test_mode"`
}

// HandleCreate: POST /api/admin/events. Auto-assigns the creator as event_admin.
func (s *Service) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	applyCreateDefaults(&req)

	if err := validateCreate(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errCode(err), err.Error())
		return
	}

	taken, err := s.pool.Queries().SlugTaken(r.Context(), req.Slug)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if taken {
		writeErr(w, http.StatusConflict, "slug_taken", "an event with this slug already exists")
		return
	}

	var created db.Event
	err = s.pool.WithTx(r.Context(), func(q *db.Queries) error {
		var err error
		var participantLimit *int32
		if req.ParticipantLimit != nil {
			v := int32(*req.ParticipantLimit)
			participantLimit = &v
		}
		created, err = q.CreateEvent(r.Context(), db.CreateEventParams{
			Slug:              req.Slug,
			Name:              strings.TrimSpace(req.Name),
			Description:       req.Description,
			BannerRef:         nil,
			ColorPrimary:      req.ColorPrimary,
			ColorSecondary:    req.ColorSecondary,
			ColorText:         req.ColorText,
			Location:          req.Location,
			StartsAt:          req.StartsAt,
			EndsAt:            req.EndsAt,
			ParticipantLimit:  participantLimit,
			PricingMode:       req.PricingMode,
			Currency:          req.Currency,
			DefaultLocale:     req.DefaultLocale,
			IsPublic:          req.IsPublic,
			CreatedBy:         user.ID,
			PaymentTiming:     req.PaymentTiming,
			BankIban:          ptrIfNonEmpty(req.BankIBAN),
			BankBic:           ptrIfNonEmpty(req.BankBIC),
			BankAccountHolder: ptrIfNonEmpty(req.BankAccountHolder),
			PaypalHandle:      ptrIfNonEmpty(req.PaypalHandle),
			PaymentTestMode:   req.PaymentTestMode,
		})
		if err != nil {
			return err
		}
		// Creator is automatically assigned as event_admin so they can mutate
		// what they just created (RBAC otherwise rejects non-global-admins).
		if user.Role != domain.RoleGlobalAdmin {
			if err := q.AssignEventAdmin(r.Context(), db.AssignEventAdminParams{
				UserID:  user.ID,
				EventID: created.ID,
			}); err != nil {
				return fmt.Errorf("auto-assign creator: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		logging.FromContext(r.Context()).Error("create event failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if err := audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &created.ID,
		Action:      audit.ActionEventCreate,
		TargetKind:  audit.TargetEvent,
		TargetID:    created.ID.String(),
		Payload:     map[string]any{"slug": created.Slug, "name": created.Name},
	}); err != nil {
		logging.FromContext(r.Context()).Warn("audit record failed", "err", err)
	}

	writeJSON(w, http.StatusCreated, toEventResponse(created))
}

func applyCreateDefaults(req *createRequest) {
	if req.ColorPrimary == "" {
		req.ColorPrimary = defaultColorPrimary
	}
	if req.ColorSecondary == "" {
		req.ColorSecondary = defaultColorSecondary
	}
	if req.ColorText == "" {
		req.ColorText = defaultColorText
	}
	if req.Currency == "" {
		req.Currency = defaultCurrency
	}
	if req.DefaultLocale == "" {
		req.DefaultLocale = defaultLocale
	}
	if req.PricingMode == "" {
		req.PricingMode = "matrix"
	}
	if req.PaymentTiming == "" {
		req.PaymentTiming = "beforehand"
	}
}

func validateCreate(req *createRequest) error {
	if err := validateSlug(req.Slug); err != nil {
		return err
	}
	if err := validateName(req.Name); err != nil {
		return err
	}
	if err := validateDates(req.StartsAt, req.EndsAt); err != nil {
		return err
	}
	if err := validateColor(req.ColorPrimary); err != nil {
		return err
	}
	if err := validateColor(req.ColorSecondary); err != nil {
		return err
	}
	if err := validateColor(req.ColorText); err != nil {
		return err
	}
	if err := validateParticipantLimit(req.ParticipantLimit); err != nil {
		return err
	}
	if err := validatePricingMode(req.PricingMode); err != nil {
		return err
	}
	if err := validateCurrency(req.Currency); err != nil {
		return err
	}
	if err := validateLocale(req.DefaultLocale); err != nil {
		return err
	}
	if err := validatePaymentTiming(req.PaymentTiming); err != nil {
		return err
	}
	return nil
}

// ============================================================================
// Update
// ============================================================================

type updateRequest struct {
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	ColorPrimary     string    `json:"color_primary"`
	ColorSecondary   string    `json:"color_secondary"`
	ColorText        string    `json:"color_text"`
	Location         string    `json:"location"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	ParticipantLimit *int      `json:"participant_limit"`
	PricingMode      string    `json:"pricing_mode"`
	Currency         string    `json:"currency"`
	DefaultLocale    string    `json:"default_locale"`

	PaymentTiming     string `json:"payment_timing"`
	BankIBAN          string `json:"bank_iban"`
	BankBIC           string `json:"bank_bic"`
	BankAccountHolder string `json:"bank_account_holder"`
	PaypalHandle      string `json:"paypal_handle"`
	PaymentTestMode   bool   `json:"payment_test_mode"`
}

// HandleUpdate: PATCH /api/admin/events/{id}.
//
// Slug is intentionally NOT mutable — it's part of public URLs once the event
// is published. is_public is changed via the dedicated publish/unpublish
// endpoints so the audit trail is explicit.
func (s *Service) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	applyUpdateDefaults(&req)
	if err := validateUpdate(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errCode(err), err.Error())
		return
	}

	var participantLimit *int32
	if req.ParticipantLimit != nil {
		v := int32(*req.ParticipantLimit)
		participantLimit = &v
	}
	updated, err := s.pool.Queries().UpdateEvent(r.Context(), db.UpdateEventParams{
		ID:                id,
		Name:              strings.TrimSpace(req.Name),
		Description:       req.Description,
		ColorPrimary:      req.ColorPrimary,
		ColorSecondary:    req.ColorSecondary,
		ColorText:         req.ColorText,
		Location:          req.Location,
		StartsAt:          req.StartsAt,
		EndsAt:            req.EndsAt,
		ParticipantLimit:  participantLimit,
		PricingMode:       req.PricingMode,
		Currency:          req.Currency,
		DefaultLocale:     req.DefaultLocale,
		PaymentTiming:     req.PaymentTiming,
		BankIban:          ptrIfNonEmpty(req.BankIBAN),
		BankBic:           ptrIfNonEmpty(req.BankBIC),
		BankAccountHolder: ptrIfNonEmpty(req.BankAccountHolder),
		PaypalHandle:      ptrIfNonEmpty(req.PaypalHandle),
		PaymentTestMode:   req.PaymentTestMode,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	s.recordEventAudit(r.Context(), user.ID, id, audit.ActionEventUpdate, map[string]any{
		"name": updated.Name,
	})
	writeJSON(w, http.StatusOK, toEventResponse(updated))
}

func applyUpdateDefaults(req *updateRequest) {
	if req.Currency == "" {
		req.Currency = defaultCurrency
	}
	if req.DefaultLocale == "" {
		req.DefaultLocale = defaultLocale
	}
	if req.PricingMode == "" {
		req.PricingMode = "matrix"
	}
	if req.ColorPrimary == "" {
		req.ColorPrimary = defaultColorPrimary
	}
	if req.ColorSecondary == "" {
		req.ColorSecondary = defaultColorSecondary
	}
	if req.ColorText == "" {
		req.ColorText = defaultColorText
	}
	if req.PaymentTiming == "" {
		req.PaymentTiming = "beforehand"
	}
}

func validateUpdate(req *updateRequest) error {
	if err := validateName(req.Name); err != nil {
		return err
	}
	if err := validateDates(req.StartsAt, req.EndsAt); err != nil {
		return err
	}
	if err := validateColor(req.ColorPrimary); err != nil {
		return err
	}
	if err := validateColor(req.ColorSecondary); err != nil {
		return err
	}
	if err := validateColor(req.ColorText); err != nil {
		return err
	}
	if err := validateParticipantLimit(req.ParticipantLimit); err != nil {
		return err
	}
	if err := validatePricingMode(req.PricingMode); err != nil {
		return err
	}
	if err := validateCurrency(req.Currency); err != nil {
		return err
	}
	if err := validateLocale(req.DefaultLocale); err != nil {
		return err
	}
	if err := validatePaymentTiming(req.PaymentTiming); err != nil {
		return err
	}
	return nil
}

// ============================================================================
// Lifecycle: archive / publish / unpublish
// ============================================================================

// HandleArchive: POST /api/admin/events/{id}/archive
func (s *Service) HandleArchive(w http.ResponseWriter, r *http.Request) {
	s.runLifecycle(w, r, audit.ActionEventArchive, func(ctx context.Context, q *db.Queries, id uuid.UUID) error {
		return q.ArchiveEvent(ctx, id)
	})
}

// HandlePublish: POST /api/admin/events/{id}/publish
func (s *Service) HandlePublish(w http.ResponseWriter, r *http.Request) {
	s.runLifecycle(w, r, audit.ActionEventPublish, func(ctx context.Context, q *db.Queries, id uuid.UUID) error {
		return q.PublishEvent(ctx, id)
	})
}

// HandleUnpublish: POST /api/admin/events/{id}/unpublish
func (s *Service) HandleUnpublish(w http.ResponseWriter, r *http.Request) {
	s.runLifecycle(w, r, audit.ActionEventUnpublish, func(ctx context.Context, q *db.Queries, id uuid.UUID) error {
		return q.UnpublishEvent(ctx, id)
	})
}

func (s *Service) runLifecycle(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	op func(context.Context, *db.Queries, uuid.UUID) error,
) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	if err := op(r.Context(), s.pool.Queries(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, id, action, nil)

	row, err := s.pool.Queries().GetEventByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEventResponse(row))
}

// ============================================================================
// Helpers
// ============================================================================

func (s *Service) recordEventAudit(ctx context.Context, actorID, eventID uuid.UUID, action string, payload map[string]any) {
	if err := audit.Record(ctx, s.pool, audit.Entry{
		ActorUserID: &actorID,
		EventID:     &eventID,
		Action:      action,
		TargetKind:  audit.TargetEvent,
		TargetID:    eventID.String(),
		Payload:     payload,
	}); err != nil {
		logging.FromContext(ctx).Warn("audit record failed", "err", err)
	}
}

// errCode maps a validation sentinel error to a stable string code suitable
// for the API error response. Falls back to "validation_error".
func errCode(err error) string {
	switch {
	case errors.Is(err, ErrInvalidSlug):
		return "invalid_slug"
	case errors.Is(err, ErrSlugTaken):
		return "slug_taken"
	case errors.Is(err, ErrInvalidName):
		return "invalid_name"
	case errors.Is(err, ErrInvalidDates):
		return "invalid_dates"
	case errors.Is(err, ErrInvalidColor):
		return "invalid_color"
	case errors.Is(err, ErrInvalidParticipantLimit):
		return "invalid_participant_limit"
	case errors.Is(err, ErrInvalidPricingMode):
		return "invalid_pricing_mode"
	case errors.Is(err, ErrInvalidCurrency):
		return "invalid_currency"
	case errors.Is(err, ErrInvalidLocale):
		return "invalid_locale"
	case errors.Is(err, ErrInvalidPaymentTiming):
		return "invalid_payment_timing"
	default:
		return "validation_error"
	}
}
