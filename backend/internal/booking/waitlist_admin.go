package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/authz"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
)

// waitlistDTO mirrors the public response shape but exposes admin-only
// fields (status, promoted_at, claim_deadline, fulfilled_booking_id).
type waitlistDTO struct {
	ID                 uuid.UUID  `json:"id"`
	ContactName        string     `json:"contact_name"`
	ContactEmail       string     `json:"contact_email"`
	Locale             string     `json:"locale"`
	RequestedSeats     int        `json:"requested_seats"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	PromotedAt         *time.Time `json:"promoted_at,omitempty"`
	ClaimDeadline      *time.Time `json:"claim_deadline,omitempty"`
	FulfilledBookingID *uuid.UUID `json:"fulfilled_booking_id,omitempty"`
}

// HandleListWaitlist: GET /api/admin/events/{id}/waitlist
//
// Returns every waitlist row for the event (all statuses) plus a counters
// strip for the summary. Only meaningful for limited events; for unlimited
// events the list is always empty, which the frontend uses to hide the tab.
func (s *Service) HandleListWaitlist(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingList, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	rows, err := s.pool.Queries().ListWaitlistForEvent(r.Context(), eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	counts, err := s.pool.Queries().WaitlistStatusCountsForEvent(r.Context(), eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]waitlistDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, waitlistDTO{
			ID:                 e.ID,
			ContactName:        e.ContactName,
			ContactEmail:       e.ContactEmail,
			Locale:             e.Locale,
			RequestedSeats:     int(e.RequestedSeats),
			Status:             e.Status,
			CreatedAt:          e.CreatedAt,
			PromotedAt:         e.PromotedAt,
			ClaimDeadline:      e.ClaimDeadline,
			FulfilledBookingID: e.FulfilledBookingID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"waitlist": out,
		"counts": map[string]any{
			"waiting":   int(counts.WaitingCount),
			"promoted":  int(counts.PromotedCount),
			"fulfilled": int(counts.FulfilledCount),
			"expired":   int(counts.ExpiredCount),
			"removed":   int(counts.RemovedCount),
		},
	})
}

// HandleAdminWaitlistPromote: POST /api/admin/waitlist/{id}/promote
//
// One-row promotion (skip-the-queue). Behavior depends on event mode:
//   - donation event: directly fulfill the waiter (create paid booking +
//     tickets from their frozen selection, send Stage 2 confirmation).
//   - paid event:     send a `waitlist.spot_opened` email with the claim
//     link and set claim_deadline. The waiter then claims via the standard
//     public claim flow.
//
// Only rows in status='waiting' or 'expired' can be promoted. Rows that are
// already fulfilled/promoted/removed return 409.
func (s *Service) HandleAdminWaitlistPromote(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	q := s.pool.Queries()
	row, err := q.GetWaitlistEntryByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "waitlist entry not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := authz.Allow(r.Context(), q, user, authz.ActionBookingMarkPaid, authz.ForEvent(row.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row.Status != "waiting" && row.Status != "expired" {
		writeErr(w, http.StatusConflict, "not_promotable", "row is "+row.Status)
		return
	}

	eventRow, err := q.GetEventByID(r.Context(), row.EventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	event := domain.EventFromDB(eventRow)

	if event.PricingMode == domain.PricingModeDonation {
		// Donation: create the paid booking inside a tx, then send Stage 2
		// outside the lock window.
		var booking db.Booking
		var participants []db.Participant
		var infos []ticketInfo
		err := s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
			if err := tx.LockEventForCapacity(r.Context(), event.ID); err != nil {
				return err
			}
			b, p, i, err := s.fulfillWaitlistDonation(r.Context(), tx, event, row)
			if err != nil {
				return err
			}
			booking, participants, infos = b, p, i
			return nil
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if err := s.sendTicketsConfirmed(r.Context(), event, booking, participants, infos); err != nil {
			logger.Warn("admin manual promote: Stage 2 email failed", "err", err, "booking_id", booking.ID)
		}
		_ = audit.Record(r.Context(), s.pool, audit.Entry{
			ActorUserID: &user.ID,
			EventID:     &event.ID,
			Action:      "waitlist.admin_promote",
			TargetKind:  audit.TargetEvent,
			TargetID:    id.String(),
			Payload:     map[string]any{"mode": "donation", "booking_id": booking.ID.String()},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"outcome":           "fulfilled",
			"booking_id":        booking.ID,
			"booking_reference": formatReference(booking.ID),
		})
		return
	}

	// Paid event: send the spot_opened email to this one waiter. The waiter
	// then races to claim via the standard public flow.
	hours := event.WaitlistClaimWindowHours
	if hours <= 0 {
		hours = 24
	}
	deadline := time.Now().Add(time.Duration(hours) * time.Hour)
	if err := q.MarkWaitlistPromoted(r.Context(), db.MarkWaitlistPromotedParams{
		ID:            id,
		ClaimDeadline: &deadline,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	openSeats := int(row.RequestedSeats)
	if err := s.sendWaitlistSpotOpened(r.Context(), event, row, openSeats); err != nil {
		logger.Warn("admin manual promote: spot_opened email failed", "err", err, "waitlist_id", id)
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &event.ID,
		Action:      "waitlist.admin_promote",
		TargetKind:  audit.TargetEvent,
		TargetID:    id.String(),
		Payload:     map[string]any{"mode": "paid", "claim_deadline": deadline.Format(time.RFC3339)},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"outcome":        "promoted",
		"claim_deadline": deadline.Format(time.RFC3339),
	})
}

// HandleAdminWaitlistRemove: POST /api/admin/waitlist/{id}/remove
//
// Admin sets the row to status='removed' and sends a courtesy self-removal
// email so the waiter knows. Idempotent on already-removed/expired rows.
type adminWaitlistRemoveRequest struct {
	NotifyUser bool `json:"notify_user"`
}

func (s *Service) HandleAdminWaitlistRemove(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	// notify_user defaults to true. Optional body lets the admin suppress
	// the email for spam / test rows.
	notify := true
	if r.Body != nil && r.ContentLength > 0 {
		var req adminWaitlistRemoveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			notify = req.NotifyUser
		}
	}

	q := s.pool.Queries()
	row, err := q.GetWaitlistEntryByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "waitlist entry not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := authz.Allow(r.Context(), q, user, authz.ActionBookingMarkPaid, authz.ForEvent(row.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row.Status == "fulfilled" {
		writeErr(w, http.StatusConflict, "already_fulfilled", "row has been fulfilled into a booking — refund the booking instead")
		return
	}
	if err := q.MarkWaitlistRemoved(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &row.EventID,
		Action:      "waitlist.admin_remove",
		TargetKind:  audit.TargetEvent,
		TargetID:    id.String(),
		Payload:     map[string]any{"notify_user": notify, "prior_status": row.Status},
	})

	if notify {
		eventRow, err := q.GetEventByID(r.Context(), row.EventID)
		if err == nil {
			if mailErr := s.sendWaitlistRemovedSelf(r.Context(), domain.EventFromDB(eventRow), row); mailErr != nil {
				logger.Warn("admin remove: courtesy email failed", "err", mailErr, "waitlist_id", id)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// _ avoids unused-import warnings during partial builds.
var _ context.Context
