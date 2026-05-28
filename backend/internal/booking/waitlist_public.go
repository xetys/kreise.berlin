package booking

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
	"github.com/dsteiman/tickets-general/backend/internal/pricing"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// HandleWaitlistClaim — POST /api/waitlist/claim/{token}
//
// Verifies the HMAC token, locks the event row, re-checks remaining capacity,
// atomically marks the waitlist row 'promoted' (or 'fulfilled' on immediate
// success), and creates the corresponding booking. Returns the booking
// reference so the frontend can redirect to /booked.
//
// Race semantics: if multiple waiters click their claim links simultaneously,
// the event-row FOR UPDATE serializes the inner steps; only the first to
// land inside the lock with sufficient remaining capacity wins. Subsequent
// claimers get 409 with error="claim_unavailable".
func (s *Service) HandleWaitlistClaim(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())
	token := chi.URLParam(r, "token")
	if token == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "token required")
		return
	}
	entryID, _, err := tokens.Verify(tokens.PurposeWaitlistClaim, token, s.cfg.TokenSigningKey)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}

	q := s.pool.Queries()
	row, err := q.GetWaitlistEntryByID(r.Context(), entryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "waitlist entry not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row.Status == "fulfilled" {
		// Idempotent: same waiter clicking the same link twice gets the
		// existing booking reference back.
		ref := ""
		if row.FulfilledBookingID != nil {
			ref = formatReference(*row.FulfilledBookingID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"outcome": "already_claimed", "booking_reference": ref})
		return
	}
	if row.Status == "removed" || row.Status == "expired" {
		writeErr(w, http.StatusConflict, "claim_unavailable", "this waitlist entry is no longer active")
		return
	}

	eventRow, err := q.GetEventByID(r.Context(), row.EventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	event := domain.EventFromDB(eventRow)

	var booking db.Booking
	var participantsCreated []db.Participant
	var ticketInfos []ticketInfo

	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		if err := tx.LockEventForCapacity(r.Context(), event.ID); err != nil {
			return fmt.Errorf("lock event: %w", err)
		}
		seats, err := tx.CountActiveSeatsForEvent(r.Context(), event.ID)
		if err != nil {
			return fmt.Errorf("count seats: %w", err)
		}
		remaining := *event.ParticipantLimit - int(seats)
		if remaining < int(row.RequestedSeats) {
			return errClaimUnavailable
		}

		// Re-load row inside the lock to catch a concurrent self-removal /
		// admin removal between the upfront read and the lock acquire.
		fresh, err := tx.GetWaitlistEntryByID(r.Context(), row.ID)
		if err != nil {
			return err
		}
		if fresh.Status == "fulfilled" || fresh.Status == "removed" || fresh.Status == "expired" {
			return errClaimUnavailable
		}

		// Build a paid booking from the frozen quote. Paid event waitlist
		// claims become a tentative 'booked' status with reservation_expires_at
		// to give the holder the configured claim window to actually pay
		// (bank transfer / PayPal); reservation expiry then re-promotes.
		// Donation/at_door/test events are short-circuited through the
		// promote-on-cancel path before getting here, so claim links are
		// only sent on paid events with payment_timing=beforehand.
		var sel WaitlistSelection
		if err := json.Unmarshal(fresh.SelectionJson, &sel); err != nil {
			return fmt.Errorf("unmarshal selection: %w", err)
		}
		hours := event.WaitlistClaimWindowHours
		if hours <= 0 {
			hours = 24
		}
		expiry := time.Now().Add(time.Duration(hours) * time.Hour)
		ref := newPaymentReference()
		bookingRow, err := tx.CreateBooking(r.Context(), db.CreateBookingParams{
			EventID:              event.ID,
			ContactEmail:         fresh.ContactEmail,
			ContactName:          fresh.ContactName,
			ContactPhone:         nilIfEmpty(sel.Phone),
			Locale:               fresh.Locale,
			Status:               "booked",
			TotalAmountMinor:     sel.Quote.TotalMinor,
			Currency:             sel.Quote.Currency,
			CouponID:             fresh.CouponID,
			PaymentMethod:        nil,
			PaymentReference:     &ref,
			ReservationExpiresAt: &expiry,
			PaidAt:               nil,
		})
		if err != nil {
			return fmt.Errorf("create booking: %w", err)
		}
		booking = bookingRow

		for i, p := range sel.Participants {
			pRow, err := tx.CreateParticipant(r.Context(), db.CreateParticipantParams{
				EventID:         event.ID,
				BookingID:       bookingRow.ID,
				Name:            p.Name,
				Email:           nilIfEmpty(p.Email),
				NewsletterOptin: sel.NewsletterOptin,
				Notes:           nil,
			})
			if err != nil {
				return fmt.Errorf("create participant %d: %w", i, err)
			}
			participantsCreated = append(participantsCreated, pRow)

			if i >= len(sel.Quote.LineItems) {
				return fmt.Errorf("frozen quote line item %d missing", i)
			}
			li := sel.Quote.LineItems[i]
			ticketID := uuid.New()
			nonce := make([]byte, 16)
			if _, err := rand.Read(nonce); err != nil {
				return fmt.Errorf("nonce: %w", err)
			}
			qrToken, err := tokens.Sign(tokens.PurposeQR, ticketID, nonce, s.cfg.TokenSigningKey)
			if err != nil {
				return fmt.Errorf("sign qr token: %w", err)
			}
			_, err = tx.CreateTicket(r.Context(), db.CreateTicketParams{
				ID:            ticketID,
				BookingID:     bookingRow.ID,
				ParticipantID: pRow.ID,
				EventID:       event.ID,
				Status:        "booked",
				QrToken:       qrToken,
				QrNonce:       nonce,
				PhaseID:       li.PhaseID,
				CategoryID:    li.CategoryID,
				DurationID:    li.DurationID,
				AmountMinor:   li.AmountMinor,
				PaidAt:        nil,
			})
			if err != nil {
				return fmt.Errorf("create ticket %d: %w", i, err)
			}
			ticketInfos = append(ticketInfos, ticketInfo{ID: ticketID, Nonce: nonce, ParticipantName: pRow.Name})
		}

		// Coupon redemption — best effort. Coupon stays valid for join-time
		// waiters per the locked design decision; a failure here doesn't
		// undo the booking.
		if sel.Quote.AppliedCouponCode != "" {
			_ = pricing.RedeemCoupon(r.Context(), tx, event.ID, bookingRow.ID, sel.Quote.AppliedCouponCode, fresh.ContactEmail)
		}

		// Mark fulfilled (booking is created; the waiter still has to pay
		// within the claim window, but the seat is theirs and counts against
		// capacity now).
		if err := tx.MarkWaitlistFulfilled(r.Context(), db.MarkWaitlistFulfilledParams{
			ID:                 fresh.ID,
			FulfilledBookingID: &bookingRow.ID,
		}); err != nil {
			return fmt.Errorf("mark fulfilled: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errClaimUnavailable) {
			writeErr(w, http.StatusConflict, "claim_unavailable", "the spot has already been taken")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Stage-1 email: the holder still needs to pay (claim creates a tentative
	// booking on paid events). Mirror the regular Stage-1 receipt.
	if err := s.sendRegistrationReceipt(r.Context(), event, booking, participantsCreated, ticketInfos); err != nil {
		logger.Warn("post-claim Stage-1 email failed", "err", err, "booking_id", booking.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"outcome":           "claimed",
		"booking_reference": formatReference(booking.ID),
		"booking_id":        booking.ID,
		"event_slug":        event.Slug,
		"locale":            booking.Locale,
	})
}

// HandleWaitlistRemove — POST /api/waitlist/remove/{token}
//
// Verifies the HMAC token, idempotently marks the row 'removed', sends a
// courtesy confirmation email. Always returns 200 on a valid token, even on
// already-removed rows (idempotent + uniform from the user's perspective).
func (s *Service) HandleWaitlistRemove(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())
	token := chi.URLParam(r, "token")
	if token == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "token required")
		return
	}
	entryID, _, err := tokens.Verify(tokens.PurposeWaitlistRemove, token, s.cfg.TokenSigningKey)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}

	q := s.pool.Queries()
	row, err := q.GetWaitlistEntryByID(r.Context(), entryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "waitlist entry not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row.Status == "fulfilled" {
		// Already became a real booking — can't unwind here.
		writeErr(w, http.StatusConflict, "already_fulfilled", "you've already claimed your spot — contact the organizer to cancel")
		return
	}

	if err := q.MarkWaitlistRemoved(r.Context(), row.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	eventRow, err := q.GetEventByID(r.Context(), row.EventID)
	if err == nil {
		if mailErr := s.sendWaitlistRemovedSelf(r.Context(), domain.EventFromDB(eventRow), row); mailErr != nil {
			logger.Warn("waitlist removal email failed", "err", mailErr, "id", row.ID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"outcome": "removed"})
}

var errClaimUnavailable = errors.New("claim_unavailable")

// _ avoids unused import in early builds.
var _ context.Context
