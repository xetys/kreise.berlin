package booking

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

type bookingDTO struct {
	ID                   uuid.UUID `json:"id"`
	ContactEmail         string    `json:"contact_email"`
	ContactName          string    `json:"contact_name"`
	Status               string    `json:"status"`
	TotalAmountMinor     int64     `json:"total_amount_minor"`
	Currency             string    `json:"currency"`
	PaymentMethod        string    `json:"payment_method,omitempty"`
	PaymentReference     string    `json:"payment_reference,omitempty"`
	ParticipantCount     int       `json:"participant_count"`
	ReservationExpiresAt string    `json:"reservation_expires_at,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	PaidAt               string    `json:"paid_at,omitempty"`
	Reference            string    `json:"reference"`
}

// HandleListBookings: GET /api/admin/events/{id}/bookings
func (s *Service) HandleListBookings(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	eventID, err := uuid.Parse(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}

	// Filters: every parameter is optional. Empty strings → no filter.
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	paymentMethod := strings.TrimSpace(r.URL.Query().Get("payment_method"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "created_at_desc"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	statusFilter := nilIfBlank(status)
	pmFilter := nilIfBlank(paymentMethod)
	qFilter := nilIfBlank(q)
	fromFilter, err := parseTimeOptional(from)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_from", err.Error())
		return
	}
	toFilter, err := parseTimeOptional(to)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_to", err.Error())
		return
	}

	rows, err := s.pool.Queries().SearchBookingsForEvent(r.Context(), db.SearchBookingsForEventParams{
		EventID:       eventID,
		Status:        statusFilter,
		PaymentMethod: pmFilter,
		FromTs:        fromFilter,
		ToTs:          toFilter,
		Q:             qFilter,
		Sort:          sort,
		LimitN:        int32(limit),
		OffsetN:       int32(offset),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	summary, err := s.pool.Queries().CountSearchBookingsForEvent(r.Context(), db.CountSearchBookingsForEventParams{
		EventID:       eventID,
		Status:        statusFilter,
		PaymentMethod: pmFilter,
		FromTs:        fromFilter,
		ToTs:          toFilter,
		Q:             qFilter,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	out := make([]bookingDTO, 0, len(rows))
	for _, b := range rows {
		dto := bookingDTO{
			ID:               b.ID,
			ContactEmail:     b.ContactEmail,
			ContactName:      b.ContactName,
			Status:           b.Status,
			TotalAmountMinor: b.TotalAmountMinor,
			Currency:         b.Currency,
			ParticipantCount: int(b.ParticipantCount),
			CreatedAt:        b.CreatedAt,
			Reference:        formatReference(b.ID),
		}
		if b.PaymentMethod != nil {
			dto.PaymentMethod = *b.PaymentMethod
		}
		if b.PaymentReference != nil {
			dto.PaymentReference = *b.PaymentReference
		}
		if b.ReservationExpiresAt != nil {
			dto.ReservationExpiresAt = b.ReservationExpiresAt.Format(time.RFC3339)
		}
		if b.PaidAt != nil {
			dto.PaidAt = b.PaidAt.Format(time.RFC3339)
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookings": out,
		"total":    int(summary.TotalCount),
		"limit":    limit,
		"offset":   offset,
		"summary": map[string]any{
			"total_count":         int(summary.TotalCount),
			"paid_count":          int(summary.PaidCount),
			"booked_count":        int(summary.BookedCount),
			"canceled_count":      int(summary.CanceledCount),
			"paid_revenue_minor":  summary.PaidRevenueMinor,
			"total_participants":  int(summary.TotalParticipants),
			"paid_participants":   int(summary.PaidParticipants),
			"booked_participants": int(summary.BookedParticipants),
		},
	})
}

func nilIfBlank(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func parseTimeOptional(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	// Accept either RFC3339 or YYYY-MM-DD (frontend date picker style).
	if len(s) == 10 {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q (expect YYYY-MM-DD)", s)
		}
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp %q (expect RFC3339)", s)
	}
	return &t, nil
}

// HandleMarkPaid: POST /api/admin/bookings/{id}/mark-paid
//
// Idempotent: if the booking is already paid, returns 200 with the current
// state and skips the email. Otherwise flips booking + tickets to paid and
// fires the Stage 2 confirmation email.
func (s *Service) HandleMarkPaid(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_booking_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	q := s.pool.Queries()

	booking, err := q.GetBookingByID(r.Context(), bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "booking not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Authorize against the booking's event.
	if err := authz.Allow(r.Context(), q, user, authz.ActionBookingMarkPaid, authz.ForEvent(booking.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "you do not have permission to mark-paid")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if booking.Status == "paid" {
		// Idempotent: already paid, no action.
		writeJSON(w, http.StatusOK, map[string]any{"status": booking.Status, "already_paid": true})
		return
	}
	if booking.Status == "canceled" {
		writeErr(w, http.StatusConflict, "cannot_mark_paid", "booking is canceled")
		return
	}

	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		if err := tx.MarkBookingPaid(r.Context(), db.MarkBookingPaidParams{
			ID:            bookingID,
			PaymentMethod: ptr("bank_transfer"),
		}); err != nil {
			return err
		}
		return tx.MarkBookingTicketsPaid(r.Context(), bookingID)
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Audit
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &booking.EventID,
		Action:      audit.ActionBookingMarkPaid,
		TargetKind:  audit.TargetBooking,
		TargetID:    bookingID.String(),
		Payload:     map[string]any{"reference": formatReference(bookingID)},
	})

	// Send Stage 2 email.
	freshBooking, err := q.GetBookingByID(r.Context(), bookingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	eventRow, err := q.GetEventByID(r.Context(), freshBooking.EventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	participants, err := q.ListParticipantsForBooking(r.Context(), bookingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	tickets, err := q.ListActiveTicketsForBooking(r.Context(), bookingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	infos := make([]ticketInfo, 0, len(tickets))
	for _, t := range tickets {
		var name string
		for _, p := range participants {
			if p.ID == t.ParticipantID {
				name = p.Name
				break
			}
		}
		infos = append(infos, ticketInfo{ID: t.ID, Nonce: t.QrNonce, ParticipantName: name})
	}

	if err := s.sendTicketsConfirmed(r.Context(), domain.EventFromDB(eventRow), freshBooking, participants, infos); err != nil {
		logger.Warn("Stage 2 email send failed", "err", err, "booking_id", bookingID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "paid", "already_paid": false})
}

func ptr(s string) *string { return &s }

// HandlePurgeTestBookings: POST /api/admin/events/{id}/bookings/purge-test
//
// Deletes every booking on this event with payment_method='test' (and
// cascades the participants + tickets + coupon redemptions). Useful before
// flipping test_mode off and going live, so the dashboard doesn't show
// pollution from rehearsals.
func (s *Service) HandlePurgeTestBookings(w http.ResponseWriter, r *http.Request) {
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
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionEventUpdate, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "you do not have permission to purge test bookings")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	rows, err := s.pool.Queries().PurgeTestBookingsForEvent(r.Context(), eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &eventID,
		Action:      "booking.purge_test",
		TargetKind:  audit.TargetEvent,
		TargetID:    eventID.String(),
		Payload:     map[string]any{"deleted_count": rows},
	})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": rows})
}

// HandleWaitlistPromote: POST /api/admin/events/{id}/waitlist/promote
//
// Manually triggers waitlist promotion for an event. Useful as an admin
// "rotate now" lever — also covers cases where capacity opened due to a
// path that didn't auto-fire the hook (e.g. data fixed in SQL, or a
// pre-Phase-10 cancel that never had the hook attached).
//
// Gated on ActionBookingMarkPaid (not ActionEventUpdate) — rotating the
// waitlist is operational booking management, not event metadata editing.
// That lets door managers re-rotate on-site when no-shows free a seat.
func (s *Service) HandleWaitlistPromote(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

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
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingMarkPaid, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "you do not have permission")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if err := s.PromoteAfterCancel(r.Context(), eventID, logger); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleRefund: POST /api/admin/bookings/{id}/refund
//
// Marks the booking + tickets canceled and emails the holder. Does NOT
// move money — the admin is responsible for the actual refund (PayPal.me /
// wire transfer); the platform never touched the funds.
func (s *Service) HandleRefund(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_booking_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	q := s.pool.Queries()
	booking, err := q.GetBookingByID(r.Context(), bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "booking not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := authz.Allow(r.Context(), q, user, authz.ActionBookingRefund, authz.ForEvent(booking.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "you do not have permission to refund")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if booking.Status == "canceled" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "canceled", "already_canceled": true})
		return
	}

	if err := q.CancelBookingAndTickets(r.Context(), bookingID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &booking.EventID,
		Action:      audit.ActionBookingRefund,
		TargetKind:  audit.TargetBooking,
		TargetID:    bookingID.String(),
		Payload:     map[string]any{"reference": formatReference(bookingID)},
	})

	// Best-effort refund-notification email.
	if err := s.sendRefundNotice(r.Context(), booking); err != nil {
		logger.Warn("refund email send failed", "err", err, "booking_id", bookingID)
	}

	// Trigger waitlist rotation in the background — capacity has just freed
	// up, so the next fitting waiter should be promoted (donation events) or
	// notified (paid events). Decoupled from the response so the admin's
	// refund click isn't blocked on email fan-out.
	eventID := booking.EventID
	go func() {
		if err := s.PromoteAfterCancel(context.Background(), eventID, logger); err != nil {
			logger.Warn("waitlist promotion after refund failed", "err", err, "event_id", eventID)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]any{"status": "canceled", "already_canceled": false})
}

// HandleResendConfirmation: POST /api/admin/bookings/{id}/resend-confirmation
//
// Re-fires the appropriate stage email for an existing booking. Useful when a
// booker reports the original mail went to spam or got lost. Stage depends on
// the booking's current status: Stage 1 receipt for `booked`, Stage 2 (with
// inline QR) for `paid`. canceled bookings are not eligible.
func (s *Service) HandleResendConfirmation(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_booking_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	q := s.pool.Queries()
	booking, err := q.GetBookingByID(r.Context(), bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "booking not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := authz.Allow(r.Context(), q, user, authz.ActionBookingList, authz.ForEvent(booking.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if booking.Status == "canceled" {
		writeErr(w, http.StatusConflict, "booking_canceled", "cannot resend on canceled bookings")
		return
	}

	eventRow, err := q.GetEventByID(r.Context(), booking.EventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	event := domain.EventFromDB(eventRow)
	participants, err := q.ListParticipantsForBooking(r.Context(), bookingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if booking.Status == "paid" {
		tickets, err := q.ListActiveTicketsForBooking(r.Context(), bookingID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		infos := make([]ticketInfo, 0, len(tickets))
		for _, t := range tickets {
			var name string
			for _, p := range participants {
				if p.ID == t.ParticipantID {
					name = p.Name
					break
				}
			}
			infos = append(infos, ticketInfo{ID: t.ID, Nonce: t.QrNonce, ParticipantName: name})
		}
		if err := s.sendTicketsConfirmed(r.Context(), event, booking, participants, infos); err != nil {
			logger.Warn("resend Stage 2 failed", "err", err, "booking_id", bookingID)
			writeErr(w, http.StatusInternalServerError, "email_failed", err.Error())
			return
		}
	} else {
		// booked: Stage 1 receipt. No ticket infos needed (no QR yet).
		if err := s.sendRegistrationReceipt(r.Context(), event, booking, participants, nil); err != nil {
			logger.Warn("resend Stage 1 failed", "err", err, "booking_id", bookingID)
			writeErr(w, http.StatusInternalServerError, "email_failed", err.Error())
			return
		}
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &booking.EventID,
		Action:      "booking.resend_confirmation",
		TargetKind:  audit.TargetBooking,
		TargetID:    bookingID.String(),
		Payload:     map[string]any{"status": booking.Status},
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stage": ifThen(booking.Status == "paid", "2", "1")})
}

// HandleEditContactEmail: PATCH /api/admin/bookings/{id}
//
// MVP only supports flipping contact_email (typo fix). Reusable later for
// other narrow edits — keep the surface tight to avoid the "admin patches
// everything" footgun.
type editBookingRequest struct {
	ContactEmail *string `json:"contact_email,omitempty"`
}

func (s *Service) HandleEditBooking(w http.ResponseWriter, r *http.Request) {
	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_booking_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	var req editBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.ContactEmail == nil {
		writeErr(w, http.StatusBadRequest, "no_changes", "contact_email is required")
		return
	}
	newEmail := strings.TrimSpace(strings.ToLower(*req.ContactEmail))
	if !strings.Contains(newEmail, "@") {
		writeErr(w, http.StatusBadRequest, "invalid_email", "must contain @")
		return
	}

	q := s.pool.Queries()
	booking, err := q.GetBookingByID(r.Context(), bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "booking not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := authz.Allow(r.Context(), q, user, authz.ActionBookingMarkPaid, authz.ForEvent(booking.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	oldEmail := booking.ContactEmail
	if err := q.UpdateBookingContactEmail(r.Context(), db.UpdateBookingContactEmailParams{
		ID:           bookingID,
		ContactEmail: newEmail,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &booking.EventID,
		Action:      "booking.edit_contact_email",
		TargetKind:  audit.TargetBooking,
		TargetID:    bookingID.String(),
		Payload:     map[string]any{"old": oldEmail, "new": newEmail},
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "contact_email": newEmail})
}

// HandleBookingsCSV: GET /api/admin/events/{id}/bookings/export.csv
//
// Streams a CSV of the current filter (same query params as HandleListBookings)
// without pagination. Per-row participant names are pipe-joined so each row
// stays one CSV record. Caller is responsible for writing to a file; we set
// Content-Disposition so browsers prompt a download.
func (s *Service) HandleBookingsCSV(w http.ResponseWriter, r *http.Request) {
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

	statusFilter := nilIfBlank(r.URL.Query().Get("status"))
	pmFilter := nilIfBlank(r.URL.Query().Get("payment_method"))
	qFilter := nilIfBlank(r.URL.Query().Get("q"))
	fromFilter, _ := parseTimeOptional(r.URL.Query().Get("from"))
	toFilter, _ := parseTimeOptional(r.URL.Query().Get("to"))

	rows, err := s.pool.Queries().ListExpandedBookingsForEvent(r.Context(), db.ListExpandedBookingsForEventParams{
		EventID:       eventID,
		Status:        statusFilter,
		PaymentMethod: pmFilter,
		FromTs:        fromFilter,
		ToTs:          toFilter,
		Q:             qFilter,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="bookings-%s-%s.csv"`, eventID.String()[:8], time.Now().UTC().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{
		"reference", "status", "contact_name", "contact_email",
		"total_eur", "currency", "payment_method",
		"paid_at_iso", "created_at_iso",
		"participant_count", "participant_names",
	})
	for _, b := range rows {
		paidAt := ""
		if b.PaidAt != nil {
			paidAt = b.PaidAt.UTC().Format(time.RFC3339)
		}
		pm := ""
		if b.PaymentMethod != nil {
			pm = *b.PaymentMethod
		}
		_ = cw.Write([]string{
			formatReference(b.ID),
			b.Status,
			b.ContactName,
			b.ContactEmail,
			fmt.Sprintf("%.2f", float64(b.TotalAmountMinor)/100.0),
			b.Currency,
			pm,
			paidAt,
			b.CreatedAt.UTC().Format(time.RFC3339),
			fmt.Sprintf("%d", b.ParticipantCount),
			b.ParticipantNames,
		})
	}
}

// HandleBulkMarkPaid: POST /api/admin/bookings/bulk-mark-paid
//
// Accepts {event_id, booking_ids: [...]}. For each id: verifies it belongs to
// the event, RBAC allows mark_paid, then runs the same path as HandleMarkPaid.
// Per-row failures don't poison the batch — response shape is
// {succeeded: [...], failed: [{id, reason}]}.
type bulkMarkPaidRequest struct {
	EventID    uuid.UUID   `json:"event_id"`
	BookingIDs []uuid.UUID `json:"booking_ids"`
}

func (s *Service) HandleBulkMarkPaid(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	var req bulkMarkPaidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.EventID == uuid.Nil || len(req.BookingIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request", "event_id and booking_ids are required")
		return
	}
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingMarkPaid, authz.ForEvent(req.EventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	type failure struct {
		ID     uuid.UUID `json:"id"`
		Reason string    `json:"reason"`
	}
	succeeded := make([]uuid.UUID, 0, len(req.BookingIDs))
	failed := make([]failure, 0)

	q := s.pool.Queries()
	for _, id := range req.BookingIDs {
		booking, err := q.GetBookingByID(r.Context(), id)
		if err != nil {
			failed = append(failed, failure{ID: id, Reason: "not_found"})
			continue
		}
		if booking.EventID != req.EventID {
			failed = append(failed, failure{ID: id, Reason: "event_mismatch"})
			continue
		}
		if booking.Status == "paid" {
			succeeded = append(succeeded, id) // idempotent
			continue
		}
		if booking.Status == "canceled" {
			failed = append(failed, failure{ID: id, Reason: "canceled"})
			continue
		}
		err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
			if err := tx.MarkBookingPaid(r.Context(), db.MarkBookingPaidParams{
				ID:            id,
				PaymentMethod: ptr("bank_transfer"),
			}); err != nil {
				return err
			}
			return tx.MarkBookingTicketsPaid(r.Context(), id)
		})
		if err != nil {
			failed = append(failed, failure{ID: id, Reason: err.Error()})
			continue
		}
		_ = audit.Record(r.Context(), s.pool, audit.Entry{
			ActorUserID: &user.ID,
			EventID:     &booking.EventID,
			Action:      audit.ActionBookingMarkPaid,
			TargetKind:  audit.TargetBooking,
			TargetID:    id.String(),
			Payload:     map[string]any{"bulk": true},
		})
		// Stage 2 email — best effort.
		freshBooking, err := q.GetBookingByID(r.Context(), id)
		if err == nil {
			eventRow, err := q.GetEventByID(r.Context(), freshBooking.EventID)
			if err == nil {
				participants, _ := q.ListParticipantsForBooking(r.Context(), id)
				tickets, _ := q.ListActiveTicketsForBooking(r.Context(), id)
				infos := make([]ticketInfo, 0, len(tickets))
				for _, t := range tickets {
					var name string
					for _, p := range participants {
						if p.ID == t.ParticipantID {
							name = p.Name
							break
						}
					}
					infos = append(infos, ticketInfo{ID: t.ID, Nonce: t.QrNonce, ParticipantName: name})
				}
				if err := s.sendTicketsConfirmed(r.Context(), domain.EventFromDB(eventRow), freshBooking, participants, infos); err != nil {
					logger.Warn("bulk mark-paid Stage 2 failed", "err", err, "booking_id", id)
				}
			}
		}
		succeeded = append(succeeded, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"succeeded": succeeded,
		"failed":    failed,
	})
}

func ifThen[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
