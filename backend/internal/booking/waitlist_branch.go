package booking

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
)

// WaitlistSelection is the JSON payload stored in waitlist_entries.selection_json.
// It freezes everything needed to reconstruct the booking at claim time without
// re-running pricing — coupon stays valid through claim per the locked design
// decision.
type WaitlistSelection struct {
	Participants    []ParticipantInput `json:"participants"`
	Phone           string             `json:"phone,omitempty"`
	NewsletterOptin bool               `json:"newsletter_optin"`
	Locale          string             `json:"locale,omitempty"`
	CouponCode      string             `json:"coupon_code,omitempty"`
	Quote           QuoteResponse      `json:"quote"`
}

// createWaitlistEntry runs inside the booking transaction once the capacity
// check has rejected the booking. It freezes the request + computed quote
// onto the waitlist row so the claim flow can reconstruct the booking at
// the original price (locked-in coupon).
//
// Returns the created row plus the waiter's 1-based position and the total
// number of waiting rows after the insert (used in the "joined" email).
func (s *Service) createWaitlistEntry(
	ctx context.Context,
	tx *db.Queries,
	event domain.Event,
	req *BookingRequest,
	quote QuoteResponse,
) (db.WaitlistEntry, int, int, error) {
	sel := WaitlistSelection{
		Participants:    req.Participants,
		Phone:           req.Contact.Phone,
		NewsletterOptin: req.NewsletterOptin,
		Locale:          normalizeLocale(req.Locale),
		CouponCode:      quote.AppliedCouponCode,
		Quote:           quote,
	}
	selJSON, err := json.Marshal(sel)
	if err != nil {
		return db.WaitlistEntry{}, 0, 0, fmt.Errorf("marshal selection: %w", err)
	}

	var couponID *uuid.UUID
	if quote.AppliedCouponID != nil {
		c := *quote.AppliedCouponID
		couponID = &c
	}

	entry, err := tx.CreateWaitlistEntry(ctx, db.CreateWaitlistEntryParams{
		EventID:        event.ID,
		ContactEmail:   req.Contact.Email,
		ContactName:    req.Contact.Name,
		Locale:         normalizeLocale(req.Locale),
		RequestedSeats: int32(len(req.Participants)),
		SelectionJson:  selJSON,
		CouponID:       couponID,
	})
	if err != nil {
		return db.WaitlistEntry{}, 0, 0, fmt.Errorf("create waitlist entry: %w", err)
	}

	total, err := tx.CountWaitingForEvent(ctx, event.ID)
	if err != nil {
		return db.WaitlistEntry{}, 0, 0, fmt.Errorf("count waiting: %w", err)
	}
	pos, err := tx.PositionInWaitlist(ctx, db.PositionInWaitlistParams{
		EventID: event.ID,
		ID:      entry.ID,
	})
	if err != nil {
		return db.WaitlistEntry{}, 0, 0, fmt.Errorf("position: %w", err)
	}
	return entry, int(pos), int(total), nil
}

// writeWaitlistResponse sends the JSON response when a booking submission
// landed on the waitlist instead of becoming an actual booking.
func writeWaitlistResponse(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
	entry db.WaitlistEntry,
	position, total int,
) {
	logger.Info("booking submission landed on waitlist",
		"waitlist_id", entry.ID,
		"event_id", entry.EventID,
		"position", position,
		"total", total,
	)
	writeJSON(w, http.StatusCreated, BookingResponse{
		Outcome:          "waitlisted",
		WaitlistID:       entry.ID,
		WaitlistPosition: position,
		WaitlistTotal:    total,
		ParticipantCount: int(entry.RequestedSeats),
		Currency:         "", // event currency not surfaced here; frontend re-reads event
	})
	_ = r // future: per-request audit
}

// _ silence import if logging not used elsewhere
var _ = logging.FromContext
