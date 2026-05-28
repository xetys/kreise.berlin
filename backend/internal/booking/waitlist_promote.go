package booking

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/pricing"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
	"github.com/dsteiman/tickets-general/backend/internal/waitlist"
)

// PromoteAfterCancel runs the post-cancel waitlist promotion logic for an
// event. Behaviour depends on the event's pricing mode:
//
//   - Donation events: create paid bookings directly for as many fitting
//     waiters as capacity allows (FIFO, skip-if-doesn't-fit). Each gets a
//     Stage-2 confirmation email. No claim window.
//   - Paid events: send a "spot opened" email to every fitting waiter (FIFO
//     order, all in parallel). The first to click their claim link wins via
//     HandleWaitlistClaim. No state change on the waitlist row here — the
//     claim handler is what transitions a row to 'promoted'/'fulfilled'.
//
// Unlimited events (participant_limit IS NULL) short-circuit immediately.
//
// Safe to call from a goroutine — it owns its own transaction(s) and returns
// any error to the caller (typically logged best-effort).
func (s *Service) PromoteAfterCancel(ctx context.Context, eventID uuid.UUID, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	// Read event upfront (no lock yet) so we can short-circuit on unlimited
	// events without taking a tx at all.
	q := s.pool.Queries()
	eventRow, err := q.GetEventByID(ctx, eventID)
	if err != nil {
		return fmt.Errorf("load event: %w", err)
	}
	event := domain.EventFromDB(eventRow)
	if !event.HasParticipantLimit() {
		return nil
	}

	// We collect the work to do under a lock, then perform side effects
	// (emails for paid events, full booking creation for donation events)
	// after the lock is released — keeps lock duration minimal.
	type donationFulfillment struct {
		entry        db.WaitlistEntry
		seats        int
		bookingID    uuid.UUID
		ticketInfo   []ticketInfo
		participants []db.Participant
		event        domain.Event
		booking      db.Booking
	}
	var paidNotify []db.WaitlistEntry
	var donationDone []donationFulfillment
	var openSeats int

	err = s.pool.WithTx(ctx, func(tx *db.Queries) error {
		// Lock the event row to serialize concurrent promote calls.
		if err := tx.LockEventForCapacity(ctx, event.ID); err != nil {
			return fmt.Errorf("lock event: %w", err)
		}
		seats, err := tx.CountActiveSeatsForEvent(ctx, event.ID)
		if err != nil {
			return fmt.Errorf("count seats: %w", err)
		}
		remaining := *event.ParticipantLimit - int(seats)
		if remaining <= 0 {
			return nil
		}
		openSeats = remaining

		rows, err := tx.ListWaitingForEventLocking(ctx, event.ID)
		if err != nil {
			return fmt.Errorf("list waiting: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}

		// Run pure allocator. For donation events we promote directly here;
		// for paid events the allocator output is informational — we email
		// every entry whose requested_seats <= remaining (effectively the
		// same set the allocator would consume).
		waiters := make([]waitlist.Waiter, 0, len(rows))
		for _, r := range rows {
			waiters = append(waiters, waitlist.Waiter{ID: r.ID, RequestedSeats: int(r.RequestedSeats)})
		}
		promotions := waitlist.AllocateFIFO(remaining, waiters)
		if len(promotions) == 0 {
			return nil
		}

		// Index by ID so we can grab the full row from a Promotion.
		byID := make(map[uuid.UUID]db.WaitlistEntry, len(rows))
		for _, r := range rows {
			byID[r.ID] = r
		}

		if event.PricingMode == domain.PricingModeDonation {
			// Auto-promote path: actually create the bookings inside this tx.
			for _, p := range promotions {
				row := byID[p.WaiterID]
				booking, parts, infos, err := s.fulfillWaitlistDonation(ctx, tx, event, row)
				if err != nil {
					return fmt.Errorf("fulfill donation %s: %w", row.ID, err)
				}
				donationDone = append(donationDone, donationFulfillment{
					entry: row, seats: p.Seats, bookingID: booking.ID,
					ticketInfo: infos, participants: parts,
					event: event, booking: booking,
				})
			}
			return nil
		}

		// Paid path: collect the entries to notify; no DB state change here
		// (the claim handler is what transitions). We notify everyone whose
		// requested_seats fits in remaining — including waiters the allocator
		// didn't pick, since they're all racing for the available seats.
		for _, r := range rows {
			if int(r.RequestedSeats) <= remaining {
				paidNotify = append(paidNotify, r)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Side effects outside the tx.
	for _, d := range donationDone {
		if err := s.sendTicketsConfirmed(ctx, d.event, d.booking, d.participants, d.ticketInfo); err != nil {
			logger.Warn("waitlist donation Stage-2 email failed", "err", err, "booking_id", d.booking.ID, "waitlist_id", d.entry.ID)
		}
		logger.Info("waitlist donation promoted", "booking_id", d.booking.ID, "waitlist_id", d.entry.ID, "seats", d.seats)
	}
	for _, e := range paidNotify {
		if err := s.sendWaitlistSpotOpened(ctx, event, e, openSeats); err != nil {
			logger.Warn("waitlist spot_opened email failed", "err", err, "waitlist_id", e.ID)
		} else {
			logger.Info("waitlist spot_opened email sent", "waitlist_id", e.ID, "open_seats", openSeats)
		}
	}
	return nil
}

// fulfillWaitlistDonation creates a paid booking + tickets from a waitlist
// row's frozen selection on a donation event. Marks the waitlist row
// fulfilled. Returns the new booking + participants + ticket infos so the
// caller can fire the Stage-2 email.
func (s *Service) fulfillWaitlistDonation(
	ctx context.Context,
	tx *db.Queries,
	event domain.Event,
	row db.WaitlistEntry,
) (db.Booking, []db.Participant, []ticketInfo, error) {
	var sel WaitlistSelection
	if err := json.Unmarshal(row.SelectionJson, &sel); err != nil {
		return db.Booking{}, nil, nil, fmt.Errorf("unmarshal selection: %w", err)
	}

	now := time.Now()
	pm := "donation"
	booking, err := tx.CreateBooking(ctx, db.CreateBookingParams{
		EventID:              event.ID,
		ContactEmail:         row.ContactEmail,
		ContactName:          row.ContactName,
		ContactPhone:         nilIfEmpty(sel.Phone),
		Locale:               row.Locale,
		Status:               "paid",
		TotalAmountMinor:     sel.Quote.TotalMinor,
		Currency:             sel.Quote.Currency,
		CouponID:             row.CouponID,
		PaymentMethod:        &pm,
		PaymentReference:     nil,
		ReservationExpiresAt: nil,
		PaidAt:               &now,
	})
	if err != nil {
		return db.Booking{}, nil, nil, fmt.Errorf("create booking: %w", err)
	}

	parts := make([]db.Participant, 0, len(sel.Participants))
	infos := make([]ticketInfo, 0, len(sel.Participants))
	for i, p := range sel.Participants {
		pRow, err := tx.CreateParticipant(ctx, db.CreateParticipantParams{
			EventID:         event.ID,
			BookingID:       booking.ID,
			Name:            p.Name,
			Email:           nilIfEmpty(p.Email),
			NewsletterOptin: sel.NewsletterOptin,
			Notes:           nil,
		})
		if err != nil {
			return db.Booking{}, nil, nil, fmt.Errorf("create participant %d: %w", i, err)
		}
		parts = append(parts, pRow)

		ticketID := uuid.New()
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return db.Booking{}, nil, nil, fmt.Errorf("nonce: %w", err)
		}
		qrToken, err := tokens.Sign(tokens.PurposeQR, ticketID, nonce, s.cfg.TokenSigningKey)
		if err != nil {
			return db.Booking{}, nil, nil, fmt.Errorf("sign qr token: %w", err)
		}
		if i >= len(sel.Quote.LineItems) {
			return db.Booking{}, nil, nil, fmt.Errorf("frozen quote line item %d missing", i)
		}
		li := sel.Quote.LineItems[i]
		_, err = tx.CreateTicket(ctx, db.CreateTicketParams{
			ID:            ticketID,
			BookingID:     booking.ID,
			ParticipantID: pRow.ID,
			EventID:       event.ID,
			Status:        "paid",
			QrToken:       qrToken,
			QrNonce:       nonce,
			PhaseID:       li.PhaseID,
			CategoryID:    li.CategoryID,
			DurationID:    li.DurationID,
			AmountMinor:   li.AmountMinor,
			PaidAt:        &now,
		})
		if err != nil {
			return db.Booking{}, nil, nil, fmt.Errorf("create ticket %d: %w", i, err)
		}
		infos = append(infos, ticketInfo{ID: ticketID, Nonce: nonce, ParticipantName: pRow.Name})
	}

	// Best-effort coupon redemption — see locked decision: a coupon applied
	// at waitlist-join stays valid through promotion. If the redemption count
	// has been exhausted in the meantime, log and proceed.
	if sel.Quote.AppliedCouponCode != "" {
		if err := pricing.RedeemCoupon(ctx, tx, event.ID, booking.ID, sel.Quote.AppliedCouponCode, row.ContactEmail); err != nil {
			// Don't fail the promotion — coupon was locked at join.
			_ = err
		}
	}

	if err := tx.MarkWaitlistFulfilled(ctx, db.MarkWaitlistFulfilledParams{
		ID:                 row.ID,
		FulfilledBookingID: &booking.ID,
	}); err != nil {
		return db.Booking{}, nil, nil, fmt.Errorf("mark fulfilled: %w", err)
	}

	return booking, parts, infos, nil
}
