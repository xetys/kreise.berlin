package booking

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// RunSweeper ticks every minute and cancels bookings whose
// reservation_expires_at is in the past. Cancels are durable (audit-logged
// via the audit table is left to Phase 7's waitlist hook). Returns when ctx
// is canceled.
//
// Donation-paid bookings have NULL reservation_expires_at and are unaffected.
// Unlimited-capacity events likewise have NULL — sweeper short-circuits on
// the listing query.
func (s *Service) RunSweeper(ctx context.Context, logger *slog.Logger) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()

	// Sweep once at startup so a fresh process catches missed expiries.
	s.sweepOnce(ctx, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweepOnce(ctx, logger)
		}
	}
}

func (s *Service) sweepOnce(ctx context.Context, logger *slog.Logger) {
	q := s.pool.Queries()
	rows, err := q.ListExpiredUnpaidBookings(ctx)
	if err != nil {
		logger.Warn("sweeper list failed", "err", err)
		return
	}
	if len(rows) == 0 {
		// Even with no expired bookings, expire-promoted waitlist rows whose
		// claim window passed (e.g. paid event, all waiters ignored their
		// claim email) and re-promote.
		s.expirePromotedWaitlist(ctx, logger)
		return
	}
	// Track which events had a cancel so we only run promotion once per event.
	touched := make(map[uuid.UUID]struct{}, len(rows))
	for _, b := range rows {
		if err := q.CancelBookingAndTickets(ctx, b.ID); err != nil {
			logger.Warn("sweeper cancel failed", "booking_id", b.ID, "err", err)
			continue
		}
		logger.Info("sweeper canceled expired booking", "booking_id", b.ID, "event_id", b.EventID)
		touched[b.EventID] = struct{}{}
	}
	for eventID := range touched {
		if err := s.PromoteAfterCancel(ctx, eventID, logger); err != nil {
			logger.Warn("sweeper waitlist promotion failed", "err", err, "event_id", eventID)
		}
	}
	s.expirePromotedWaitlist(ctx, logger)
}

// expirePromotedWaitlist marks 'promoted' waitlist rows whose claim_deadline
// has passed as 'expired'. The row's tentative booking (if any) was already
// canceled by the regular reservation-expiry path; here we just clean up the
// waitlist side and re-promote the next waiter.
func (s *Service) expirePromotedWaitlist(ctx context.Context, logger *slog.Logger) {
	q := s.pool.Queries()
	rows, err := q.ListExpiredPromotedWaitlist(ctx)
	if err != nil {
		logger.Warn("waitlist expiry list failed", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	touched := make(map[uuid.UUID]struct{}, len(rows))
	for _, r := range rows {
		if err := q.MarkWaitlistExpired(ctx, r.ID); err != nil {
			logger.Warn("waitlist expire failed", "err", err, "id", r.ID)
			continue
		}
		logger.Info("waitlist expired", "id", r.ID, "event_id", r.EventID)
		touched[r.EventID] = struct{}{}
	}
	for eventID := range touched {
		if err := s.PromoteAfterCancel(ctx, eventID, logger); err != nil {
			logger.Warn("waitlist re-promotion after expiry failed", "err", err, "event_id", eventID)
		}
	}
}
