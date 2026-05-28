package booking

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
	"github.com/dsteiman/tickets-general/backend/internal/pricing"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// HandleBooking: POST /api/bookings
func (s *Service) HandleBooking(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req BookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateRequest(&req); err != nil {
		writeBookingError(w, err)
		return
	}

	event, err := s.fetchPublicEvent(r.Context(), req.EventSlug)
	if err != nil {
		writeBookingError(w, err)
		return
	}

	// Compute the price authoritatively (server is source of truth).
	q, err := pricing.Compute(r.Context(), s.repo, pricing.QuoteInput{
		Event:        event,
		Selections:   toSelections(req.Participants),
		When:         time.Now(),
		CouponCode:   req.CouponCode,
		ContactEmail: req.Contact.Email,
	})
	if err != nil {
		writeBookingError(w, err)
		return
	}

	// Decide booking shape based on event mode + payment timing + test_mode.
	//
	// payment_test_mode    → immediately paid (no real money), method='test'.
	// donation             → immediately paid (donation amount is the payment).
	// matrix + at_door     → immediately paid (registration only, money on arrival).
	// matrix + beforehand  → booked, awaiting payment; reservation TTL applies.
	testMode := event.PaymentTestMode
	donation := event.PricingMode == domain.PricingModeDonation
	atDoor := event.PaymentTiming == domain.PaymentTimingAtDoor
	immediate := testMode || donation || atDoor

	status := "booked"
	var paidAt *time.Time
	var paymentMethod *string
	var reservationExpiry *time.Time
	var paymentReference *string

	if immediate {
		status = "paid"
		now := time.Now()
		paidAt = &now
		switch {
		case testMode:
			pm := "test"
			paymentMethod = &pm
		case donation:
			pm := "donation"
			paymentMethod = &pm
		case atDoor:
			pm := "at_door"
			paymentMethod = &pm
		}
	} else {
		// matrix + beforehand: hold the seats, allocate a payment reference
		// the booker can put on a wire transfer.
		exp := time.Now().Add(s.cfg.ReservationTTL)
		reservationExpiry = &exp
		ref := newPaymentReference()
		paymentReference = &ref
	}

	// All inserts + coupon redemption inside one transaction.
	var booking db.Booking
	var participantsCreated []db.Participant
	var ticketInfos []ticketInfo

	// Waitlist outcome state — populated when capacity check rejects.
	var waitlisted bool
	var waitlistEntry db.WaitlistEntry
	var waitlistPos, waitlistTotal int

	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		// Capacity check (only for limited events). Over-capacity submissions
		// branch into a waitlist entry inside the same tx — the lock taken
		// by CountActiveSeatsForEventLocking serializes this against other
		// concurrent bookings on the same event.
		if event.HasParticipantLimit() {
			seats, err := tx.CountActiveSeatsForEventLocking(r.Context(), event.ID)
			if err != nil {
				return fmt.Errorf("count seats: %w", err)
			}
			if int(seats)+len(req.Participants) > *event.ParticipantLimit {
				entry, pos, total, err := s.createWaitlistEntry(r.Context(), tx, event, &req, toQuoteResponse(q))
				if err != nil {
					return err
				}
				waitlisted = true
				waitlistEntry = entry
				waitlistPos = pos
				waitlistTotal = total
				return nil
			}
		}

		// Insert the booking.
		var couponID *uuid.UUID
		if q.AppliedCouponID != nil {
			c := *q.AppliedCouponID
			couponID = &c
		}
		bookingRow, err := tx.CreateBooking(r.Context(), db.CreateBookingParams{
			EventID:              event.ID,
			ContactEmail:         req.Contact.Email,
			ContactName:          req.Contact.Name,
			ContactPhone:         nilIfEmpty(req.Contact.Phone),
			Locale:               normalizeLocale(req.Locale),
			Status:               status,
			TotalAmountMinor:     int64(q.TotalMinor),
			Currency:             event.Currency,
			CouponID:             couponID,
			PaymentMethod:        paymentMethod,
			PaymentReference:     paymentReference,
			ReservationExpiresAt: reservationExpiry,
			PaidAt:               paidAt,
		})
		if err != nil {
			return fmt.Errorf("create booking: %w", err)
		}
		booking = bookingRow

		// Insert each participant + their ticket.
		for i, p := range req.Participants {
			pRow, err := tx.CreateParticipant(r.Context(), db.CreateParticipantParams{
				EventID:         event.ID,
				BookingID:       bookingRow.ID,
				Name:            p.Name,
				Email:           nilIfEmpty(p.Email),
				NewsletterOptin: req.NewsletterOptin,
				Notes:           nil,
			})
			if err != nil {
				return fmt.Errorf("create participant %d: %w", i, err)
			}
			participantsCreated = append(participantsCreated, pRow)

			ticketID := uuid.New()
			nonce := make([]byte, 16)
			if _, err := rand.Read(nonce); err != nil {
				return fmt.Errorf("nonce: %w", err)
			}
			qrToken, err := tokens.Sign(tokens.PurposeQR, ticketID, nonce, s.cfg.TokenSigningKey)
			if err != nil {
				return fmt.Errorf("sign qr token: %w", err)
			}
			lineItem := q.LineItems[i]
			ticketStatus := "booked"
			var ticketPaidAt *time.Time
			if immediate {
				ticketStatus = "paid"
				now := time.Now()
				ticketPaidAt = &now
			}

			_, err = tx.CreateTicket(r.Context(), db.CreateTicketParams{
				ID:            ticketID,
				BookingID:     bookingRow.ID,
				ParticipantID: pRow.ID,
				EventID:       event.ID,
				Status:        ticketStatus,
				QrToken:       qrToken,
				QrNonce:       nonce,
				PhaseID:       lineItem.PhaseID,
				CategoryID:    lineItem.CategoryID,
				DurationID:    lineItem.DurationID,
				AmountMinor:   int64(lineItem.AmountMinor),
				PaidAt:        ticketPaidAt,
			})
			if err != nil {
				return fmt.Errorf("create ticket %d: %w", i, err)
			}
			ticketInfos = append(ticketInfos, ticketInfo{ID: ticketID, Nonce: nonce, ParticipantName: pRow.Name})
		}

		// Coupon redemption — atomic with booking creation.
		if q.AppliedCouponCode != "" {
			if err := pricing.RedeemCoupon(r.Context(), tx, event.ID, bookingRow.ID, q.AppliedCouponCode, req.Contact.Email); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeBookingError(w, err)
		return
	}

	// Waitlist branch: send the "you're on the waitlist" email and return
	// the waitlist response shape. No confirmation email, no booking record.
	if waitlisted {
		if err := s.sendWaitlistJoined(r.Context(), event, waitlistEntry, waitlistPos, waitlistTotal); err != nil {
			logger.Warn("waitlist joined email failed", "err", err, slog.String("waitlist_id", waitlistEntry.ID.String()))
		}
		writeWaitlistResponse(w, r, logger, waitlistEntry, waitlistPos, waitlistTotal)
		return
	}

	// Best-effort confirmation email. Two-stage flow:
	//  - immediate (donation, at_door): Stage 2 (tickets confirmed + inline QR).
	//  - beforehand: Stage 1 (registration receipt + payment instructions).
	var sendErr error
	if immediate {
		sendErr = s.sendTicketsConfirmed(r.Context(), event, booking, participantsCreated, ticketInfos)
	} else {
		sendErr = s.sendRegistrationReceipt(r.Context(), event, booking, participantsCreated, ticketInfos)
	}
	if sendErr != nil {
		logger.Warn("booking confirmation email failed", "err", sendErr, slog.String("booking_id", booking.ID.String()))
	}

	writeJSON(w, http.StatusCreated, BookingResponse{
		Outcome:           "booked",
		BookingID:         booking.ID,
		BookingReference:  formatReference(booking.ID),
		Status:            booking.Status,
		TotalAmountMinor:  booking.TotalAmountMinor,
		Currency:          booking.Currency,
		ParticipantCount:  len(participantsCreated),
		PaymentMethod:     deref(booking.PaymentMethod),
		ReservationExpiry: derefTime(booking.ReservationExpiresAt),
	})
}

// validateRequest performs cheap structural checks; pricing rules are
// validated downstream in pricing.Compute.
func validateRequest(req *BookingRequest) error {
	if req.EventSlug == "" {
		return fmt.Errorf("%w: event_slug required", errInvalidParticipants)
	}
	if strings.TrimSpace(req.Contact.Name) == "" || !strings.Contains(req.Contact.Email, "@") {
		return errInvalidContact
	}
	if len(req.Participants) == 0 {
		return pricing.ErrNoSelections
	}
	for i, p := range req.Participants {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("%w: participant %d name required", errInvalidParticipants, i)
		}
	}
	return nil
}

// formatReference produces a short booking reference for emails / UI.
// First 8 hex chars of the UUID, uppercased.
func formatReference(id uuid.UUID) string {
	s := strings.ReplaceAll(id.String(), "-", "")
	return strings.ToUpper(s[:8])
}

// newPaymentReference produces a memorable wire-transfer reference of the
// form "TG-XXXX-XXXX" using crockford-base32-ish characters (no I/L/O/U/0/1
// to avoid ambiguity on bank statements). Length keeps it short enough that
// bookers can transcribe it manually.
func newPaymentReference() string {
	const alphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fall back to UUID-derived; reference uniqueness isn't enforced.
		return "TG-" + formatReference(uuid.New())
	}
	out := make([]byte, 8)
	for i, x := range b {
		out[i] = alphabet[int(x)%len(alphabet)]
	}
	return "TG-" + string(out[:4]) + "-" + string(out[4:])
}

// ticketInfo carries the per-ticket data that the confirmation email needs
// (specifically the nonce, so a view-purpose token can be signed and
// embedded as a magic link).
type ticketInfo struct {
	ID              uuid.UUID
	Nonce           []byte
	ParticipantName string
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefTime(p *time.Time) string {
	if p == nil {
		return ""
	}
	return p.Format(time.RFC3339)
}

func normalizeLocale(s string) string {
	switch s {
	case "de", "en":
		return s
	default:
		return "de"
	}
}

// suppress unused import warnings when individual imports aren't always referenced.
var _ = context.Background
