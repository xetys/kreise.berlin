package events

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/authz"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// HandlePublicList: GET /api/events
//
// Returns published, non-archived, future events sorted by start time.
func (s *Service) HandlePublicList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Queries().ListPublicUpcomingEvents(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]EventResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, toEventResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}

// HandlePublicGetBySlug: GET /api/events/{slug}
//
// Returns the event detail plus its full pricing config (read-only). Hidden
// for non-public or archived events. Coupons are NOT exposed in the public
// response — they're admin-only.
func (s *Service) HandlePublicGetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "invalid_slug", "slug missing")
		return
	}

	event, err := s.pool.Queries().GetEventBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !event.IsPublic || event.ArchivedAt != nil {
		writeErr(w, http.StatusNotFound, "not_found", "event not found")
		return
	}

	resp, err := s.buildPublicEventDetail(r.Context(), event)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleAdminPreview: GET /api/admin/events/{id}/preview
//
// Returns the same shape as HandlePublicGetBySlug but bypasses the
// is_public / archived gate so admins can inspect the public layout for
// drafts. Authorized via ActionEventRead — same gate as the rest of the
// admin event surface.
func (s *Service) HandleAdminPreview(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionEventRead, authz.ForEvent(id)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "you do not have permission to read this event")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	event, err := s.pool.Queries().GetEventByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp, err := s.buildPublicEventDetail(r.Context(), event)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildPublicEventDetail composes the shared response shape used by both the
// public detail endpoint and the admin preview. Caller is responsible for
// the visibility / authorization gate.
func (s *Service) buildPublicEventDetail(ctx context.Context, event db.Event) (publicEventDetail, error) {
	q := s.pool.Queries()
	resp := publicEventDetail{
		Event:      toEventResponse(event),
		Phases:     []phaseDTO{},
		Categories: []categoryDTO{},
		Durations:  []durationDTO{},
		Prices:     []priceDTO{},
	}

	if d, err := q.GetDonationConfig(ctx, event.ID); err == nil {
		resp.DonationConfig = &donationCfgDTO{SuggestedMinor: d.SuggestedMinor, MinMinor: d.MinMinor}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return publicEventDetail{}, err
	}

	phases, _ := q.ListPricePhases(ctx, event.ID)
	for _, p := range phases {
		resp.Phases = append(resp.Phases, phaseDTO{
			ID: p.ID, Name: p.Name, StartsAt: p.StartsAt, EndsAt: p.EndsAt, Ordering: int(p.Ordering),
		})
	}
	cats, _ := q.ListPriceCategories(ctx, event.ID)
	for _, c := range cats {
		resp.Categories = append(resp.Categories, categoryDTO{ID: c.ID, Name: c.Name, Ordering: int(c.Ordering)})
	}
	durs, _ := q.ListPriceDurations(ctx, event.ID)
	for _, d := range durs {
		resp.Durations = append(resp.Durations, durationDTO{ID: d.ID, Name: d.Name, Ordering: int(d.Ordering)})
	}
	prices, _ := q.ListPrices(ctx, event.ID)
	for _, p := range prices {
		resp.Prices = append(resp.Prices, priceDTO{
			ID: p.ID, PhaseID: p.PhaseID, CategoryID: p.CategoryID,
			DurationID: p.DurationID, AmountMinor: p.AmountMinor,
		})
	}

	// Program entries (public — they're part of the landing page).
	entries, _ := q.ListProgramEntries(ctx, event.ID)
	resp.Program = make([]programEntryDTO, 0, len(entries))
	for _, e := range entries {
		resp.Program = append(resp.Program, toProgramEntryDTO(e))
	}

	// Capacity-full signal — only meaningful for limited events. Unknown
	// errors are tolerated as "not full" to avoid surprising the booker.
	if event.ParticipantLimit != nil {
		if seats, err := q.CountActiveSeatsForEvent(ctx, event.ID); err == nil {
			if int(seats) >= int(*event.ParticipantLimit) {
				resp.CapacityFull = true
			}
		}
	}

	return resp, nil
}

type publicEventDetail struct {
	Event          EventResponse     `json:"event"`
	Program        []programEntryDTO `json:"program"`
	PricingMode    string            `json:"pricing_mode,omitempty"` // duplicated in Event; kept for convenience
	DonationConfig *donationCfgDTO   `json:"donation_config,omitempty"`
	Phases         []phaseDTO        `json:"phases"`
	Categories     []categoryDTO     `json:"categories"`
	Durations      []durationDTO     `json:"durations"`
	Prices         []priceDTO        `json:"prices"`

	// CapacityFull is true when the event has a participant_limit and active
	// bookings (booked + paid + checked-in) already match or exceed it. Drives
	// the pre-emptive "ausgebucht — Warteliste verfügbar" banner on the
	// public event detail page. Always false for unlimited events.
	CapacityFull bool `json:"capacity_full"`
}
