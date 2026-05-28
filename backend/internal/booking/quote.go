package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/pricing"
)

// HandleQuote: POST /api/quote
//
// Stateless. Resolves the event, runs pricing.Compute, returns the Quote.
// Used by the booking form to keep totals in sync as the user edits
// selections / coupon code. No DB writes.
func (s *Service) HandleQuote(w http.ResponseWriter, r *http.Request) {
	var req QuoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.EventSlug == "" {
		writeErr(w, http.StatusBadRequest, "missing_event_slug", "event_slug required")
		return
	}
	if len(req.Participants) == 0 {
		writeErr(w, http.StatusBadRequest, "no_selections", "at least one participant required")
		return
	}

	event, err := s.fetchPublicEvent(r.Context(), req.EventSlug)
	if err != nil {
		writeBookingError(w, err)
		return
	}

	q, err := pricing.Compute(r.Context(), s.repo, pricing.QuoteInput{
		Event:        event,
		Selections:   toSelections(req.Participants),
		When:         time.Now(),
		CouponCode:   req.CouponCode,
		ContactEmail: req.ContactEmail,
	})
	if err != nil {
		writeBookingError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteResponse(q))
}

// fetchPublicEvent loads the event by slug and validates it's bookable.
// Returns specific domain errors that writeBookingError translates.
func (s *Service) fetchPublicEvent(ctx context.Context, slug string) (domain.Event, error) {
	row, err := s.pool.Queries().GetEventBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Event{}, errEventNotFound
		}
		return domain.Event{}, err
	}
	if !row.IsPublic || row.ArchivedAt != nil {
		return domain.Event{}, errEventNotFound
	}
	if row.EndsAt.Before(time.Now()) {
		return domain.Event{}, errEventEnded
	}
	return domain.EventFromDB(row), nil
}

func toSelections(participants []ParticipantInput) []pricing.Selection {
	out := make([]pricing.Selection, 0, len(participants))
	for _, p := range participants {
		out = append(out, pricing.Selection{
			CategoryID:          p.CategoryID,
			DurationID:          p.DurationID,
			DonationAmountMinor: p.DonationAmountMinor,
		})
	}
	return out
}

func toQuoteResponse(q pricing.Quote) QuoteResponse {
	resp := QuoteResponse{
		Currency:          q.Currency,
		SubtotalMinor:     int64(q.SubtotalMinor),
		DiscountMinor:     int64(q.DiscountMinor),
		TotalMinor:        int64(q.TotalMinor),
		AppliedCouponCode: q.AppliedCouponCode,
		AppliedCouponID:   q.AppliedCouponID,
		LineItems:         make([]QuoteLineItem, 0, len(q.LineItems)),
	}
	for _, li := range q.LineItems {
		resp.LineItems = append(resp.LineItems, QuoteLineItem{
			Description: li.Description,
			AmountMinor: int64(li.AmountMinor),
			PhaseID:     li.PhaseID,
			CategoryID:  li.CategoryID,
			DurationID:  li.DurationID,
		})
	}
	if q.Phase != nil {
		resp.Phase = &QuotePhaseSummary{ID: q.Phase.ID, Name: q.Phase.Name}
	}
	return resp
}

// Suppress unused import warning when db isn't referenced from quote.go directly.
var _ = db.Booking{}
