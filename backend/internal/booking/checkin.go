package booking

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/authz"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// checkInOutcome is what the door scanner page renders. The `prior` flag
// drives the UI's "checked in just now ✓" vs. "already checked in at X ⚠"
// distinction — see HandleScan below for how it's computed.
type checkInOutcome struct {
	Outcome          string     `json:"outcome"` // "ok" | "already_checked_in" | "needs_amount"
	TicketID         uuid.UUID  `json:"ticket_id"`
	ParticipantName  string     `json:"participant_name"`
	ParticipantEmail string     `json:"participant_email,omitempty"`
	BookingReference string     `json:"booking_reference"`
	CheckedInAt      *time.Time `json:"checked_in_at,omitempty"`
	PriorCheckedInAt *time.Time `json:"prior_checked_in_at,omitempty"`
	// Door-payment fields. Populated for at_door / donation bookings; the
	// frontend uses these to render the amount-input modal before posting
	// the actual check-in.
	BookingPaymentMethod string `json:"booking_payment_method,omitempty"`
	EventPricingMode     string `json:"event_pricing_mode,omitempty"`
	ExpectedAmountMinor  int64  `json:"expected_amount_minor,omitempty"` // ticket.amount_minor — default for the input
	PaidAmountMinor      *int64 `json:"paid_amount_minor,omitempty"`     // already recorded (if re-scan)
	Currency             string `json:"currency,omitempty"`
}

// HandleScan: POST /api/admin/events/{id}/scan
//
// Body: {"token": "<qr_token>"}
//
// The QR token is the same signed value the booker has on their phone /
// emailed ticket. We:
//  1. Verify the HMAC (purpose=qr) — invalid signatures get rejected.
//  2. Check the ticket exists and belongs to THIS event (no cross-event scan
//     leakage; a manager scanning a ticket from another event gets a clear
//     error rather than a silent success).
//  3. Compare the nonce to the one stored on the ticket — guards against
//     replay of a screenshot from an old ticket whose nonce has since been
//     rotated by a transfer.
//  4. Atomically mark checked_in (idempotent — re-scan is fine and returns
//     "already_checked_in" with the original timestamp).
//
// Every attempt (success + failure) is audit-logged so the door staff's
// scanner activity is reviewable later.
type scanRequest struct {
	Token string `json:"token"`
	// Set when the staffer has confirmed / typed the door amount. If
	// missing on an at_door/donation ticket and we haven't recorded one
	// yet, the response asks the frontend to surface the amount modal.
	PaidAmountMinor *int64 `json:"paid_amount_minor,omitempty"`
	// True when the staffer has explicitly accepted "0 collected" (e.g.
	// comped guest at a donation event). Distinguishes "I forgot to enter
	// an amount" from "intentionally 0".
	ConfirmZero bool `json:"confirm_zero,omitempty"`
}

func (s *Service) HandleScan(w http.ResponseWriter, r *http.Request) {
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
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingCheckIn, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "token required")
		return
	}

	ticketID, scannedNonce, err := tokens.Verify(tokens.PurposeQR, req.Token, s.cfg.TokenSigningKey)
	if err != nil {
		s.auditScanFailure(r, user.ID, eventID, "invalid_token", req.Token, err.Error())
		writeErr(w, http.StatusUnauthorized, "invalid_token", "QR could not be verified")
		return
	}

	q := s.pool.Queries()
	row, err := q.GetTicketByID(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.auditScanFailure(r, user.ID, eventID, "not_found", req.Token, "ticket does not exist")
			writeErr(w, http.StatusNotFound, "not_found", "ticket not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row.EventID != eventID {
		s.auditScanFailure(r, user.ID, eventID, "wrong_event", req.Token, "ticket belongs to another event")
		writeErr(w, http.StatusConflict, "wrong_event", "this ticket is for a different event")
		return
	}
	// Nonce check (replay guard). subtle.ConstantTimeCompare avoids leaking
	// info via timing differences — overkill at the scan rate but cheap.
	if subtle.ConstantTimeCompare(scannedNonce, row.QrNonce) != 1 {
		s.auditScanFailure(r, user.ID, eventID, "stale_token", req.Token, "nonce mismatch (ticket was likely transferred)")
		writeErr(w, http.StatusConflict, "stale_token", "this QR has been superseded — the ticket was transferred")
		return
	}
	if row.Status == "canceled" {
		s.auditScanFailure(r, user.ID, eventID, "canceled", req.Token, "ticket is canceled")
		writeErr(w, http.StatusConflict, "canceled", "ticket has been canceled")
		return
	}
	if row.Status == "booked" {
		// Reservation but not yet paid — the door staff should decide
		// whether to accept (e.g. cash at the door) or refuse. We refuse
		// by default; the bookings page has the "mark paid" lever.
		s.auditScanFailure(r, user.ID, eventID, "unpaid", req.Token, "ticket is booked but not paid")
		writeErr(w, http.StatusConflict, "unpaid", "ticket reservation hasn't been paid yet")
		return
	}

	// Fetch participant + event context BEFORE deciding whether to ask for
	// an amount — we need booking_payment_method + event currency.
	ctx, err := q.GetTicketWithContext(r.Context(), ticketID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Does this ticket need a door-cash amount? Yes when:
	//   - the booking was paid at the door (matrix + at_door)
	//   - or the event is donation-based (= immediate-paid, but no real
	//     money received yet; staffer types the contribution)
	// On already-checked-in tickets we DON'T re-ask — the recorded amount
	// stands until someone undoes the check-in.
	bookingPaymentMethod := ""
	if ctx.BookingPaymentMethod != nil {
		bookingPaymentMethod = *ctx.BookingPaymentMethod
	}
	needsAmount := row.CheckedInAt == nil &&
		(bookingPaymentMethod == "at_door" || bookingPaymentMethod == "donation")

	if needsAmount && req.PaidAmountMinor == nil && !req.ConfirmZero {
		// Phase 1: tell the frontend to surface the amount modal. Don't
		// audit here — no state change yet. The frontend POSTs back with
		// paid_amount_minor (or confirm_zero) to complete the check-in.
		writeJSON(w, http.StatusOK, checkInOutcome{
			Outcome:              "needs_amount",
			TicketID:             ticketID,
			ParticipantName:      ctx.ParticipantName,
			BookingReference:     formatReference(ctx.BookingID),
			BookingPaymentMethod: bookingPaymentMethod,
			EventPricingMode:     ctx.EventPricingMode,
			ExpectedAmountMinor:  ctx.AmountMinor,
			Currency:             ctx.EventCurrency,
		})
		return
	}

	// Compose the amount to record. Pre-paid tickets pass nil; door tickets
	// pass either the staffer's typed value or, if confirm_zero, an
	// explicit 0.
	var amount *int64
	if needsAmount {
		if req.PaidAmountMinor != nil {
			v := *req.PaidAmountMinor
			if v < 0 {
				writeErr(w, http.StatusBadRequest, "invalid_amount", "negative amount")
				return
			}
			amount = &v
		} else if req.ConfirmZero {
			zero := int64(0)
			amount = &zero
		}
	}

	prior := row.CheckedInAt
	priorAmount := row.PaidAmountMinor
	updated, err := q.CheckInTicket(r.Context(), db.CheckInTicketParams{
		ID:              ticketID,
		PaidAmountMinor: amount,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	checkedAt := time.Now()
	if updated.CheckedInAt != nil {
		checkedAt = *updated.CheckedInAt
	}

	outcome := "ok"
	if prior != nil {
		outcome = "already_checked_in"
	}
	out := checkInOutcome{
		Outcome:              outcome,
		TicketID:             ticketID,
		ParticipantName:      ctx.ParticipantName,
		BookingReference:     formatReference(ctx.BookingID),
		CheckedInAt:          &checkedAt,
		PriorCheckedInAt:     prior,
		BookingPaymentMethod: bookingPaymentMethod,
		EventPricingMode:     ctx.EventPricingMode,
		ExpectedAmountMinor:  ctx.AmountMinor,
		PaidAmountMinor:      updated.PaidAmountMinor,
		Currency:             ctx.EventCurrency,
	}
	if ctx.ParticipantEmail != nil {
		out.ParticipantEmail = *ctx.ParticipantEmail
	}

	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &eventID,
		Action:      "ticket.check_in",
		TargetKind:  audit.TargetTicket,
		TargetID:    ticketID.String(),
		Payload: map[string]any{
			"outcome":           outcome,
			"participant":       ctx.ParticipantName,
			"booking_ref":       formatReference(ctx.BookingID),
			"already_before":    prior != nil,
			"paid_amount_minor": updated.PaidAmountMinor,
			"prior_amount":      priorAmount,
		},
	})
	logger.Info("check-in", "outcome", outcome, "ticket_id", ticketID, "participant", ctx.ParticipantName, "amount", updated.PaidAmountMinor)
	writeJSON(w, http.StatusOK, out)
}

// HandleScanManual: POST /api/admin/events/{id}/scan/manual
//
// Body: {"reference": "<8-hex>", "ticket_id": "<uuid>"}
//
// The lookup-then-confirm flow: door staff with a damaged QR types the
// booking reference, gets back the list of active tickets on that booking
// (one per participant), then re-POSTs with the chosen ticket_id to actually
// check it in. Two-phase so we don't accidentally check in the first
// participant of a 4-person booking when the staffer meant the third.
type scanManualRequest struct {
	Reference       string `json:"reference,omitempty"`
	TicketID        string `json:"ticket_id,omitempty"`
	PaidAmountMinor *int64 `json:"paid_amount_minor,omitempty"`
	ConfirmZero     bool   `json:"confirm_zero,omitempty"`
}

type manualTicketRow struct {
	ID               uuid.UUID  `json:"id"`
	ParticipantName  string     `json:"participant_name"`
	ParticipantEmail string     `json:"participant_email,omitempty"`
	BookingReference string     `json:"booking_reference"`
	CheckedInAt      *time.Time `json:"checked_in_at,omitempty"`
}

func (s *Service) HandleScanManual(w http.ResponseWriter, r *http.Request) {
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
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingCheckIn, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	var req scanManualRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Reference = strings.TrimSpace(req.Reference)
	req.TicketID = strings.TrimSpace(req.TicketID)

	q := s.pool.Queries()

	// Phase 2: confirm with a specific ticket_id → actually check in.
	if req.TicketID != "" {
		ticketID, err := uuid.Parse(req.TicketID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_ticket_id", err.Error())
			return
		}
		row, err := q.GetTicketByID(r.Context(), ticketID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found", "ticket not found")
			return
		}
		if row.EventID != eventID {
			writeErr(w, http.StatusConflict, "wrong_event", "")
			return
		}
		if row.Status == "canceled" {
			writeErr(w, http.StatusConflict, "canceled", "")
			return
		}
		if row.Status == "booked" {
			writeErr(w, http.StatusConflict, "unpaid", "")
			return
		}

		// Same two-phase shape as HandleScan: if the booking was paid at
		// the door (matrix at_door / donation) we first respond with
		// needs_amount + metadata so the UI can render the amount modal.
		ctx, err := q.GetTicketWithContext(r.Context(), ticketID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		bookingPaymentMethod := ""
		if ctx.BookingPaymentMethod != nil {
			bookingPaymentMethod = *ctx.BookingPaymentMethod
		}
		needsAmount := row.CheckedInAt == nil &&
			(bookingPaymentMethod == "at_door" || bookingPaymentMethod == "donation")
		if needsAmount && req.PaidAmountMinor == nil && !req.ConfirmZero {
			writeJSON(w, http.StatusOK, checkInOutcome{
				Outcome:              "needs_amount",
				TicketID:             ticketID,
				ParticipantName:      ctx.ParticipantName,
				BookingReference:     formatReference(ctx.BookingID),
				BookingPaymentMethod: bookingPaymentMethod,
				EventPricingMode:     ctx.EventPricingMode,
				ExpectedAmountMinor:  ctx.AmountMinor,
				Currency:             ctx.EventCurrency,
			})
			return
		}

		var amount *int64
		if needsAmount {
			if req.PaidAmountMinor != nil {
				v := *req.PaidAmountMinor
				if v < 0 {
					writeErr(w, http.StatusBadRequest, "invalid_amount", "negative amount")
					return
				}
				amount = &v
			} else if req.ConfirmZero {
				zero := int64(0)
				amount = &zero
			}
		}

		prior := row.CheckedInAt
		updated, err := q.CheckInTicket(r.Context(), db.CheckInTicketParams{
			ID:              ticketID,
			PaidAmountMinor: amount,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		checkedAt := time.Now()
		if updated.CheckedInAt != nil {
			checkedAt = *updated.CheckedInAt
		}
		outcome := "ok"
		if prior != nil {
			outcome = "already_checked_in"
		}
		out := checkInOutcome{
			Outcome:              outcome,
			TicketID:             ticketID,
			ParticipantName:      ctx.ParticipantName,
			BookingReference:     formatReference(ctx.BookingID),
			CheckedInAt:          &checkedAt,
			PriorCheckedInAt:     prior,
			BookingPaymentMethod: bookingPaymentMethod,
			EventPricingMode:     ctx.EventPricingMode,
			ExpectedAmountMinor:  ctx.AmountMinor,
			PaidAmountMinor:      updated.PaidAmountMinor,
			Currency:             ctx.EventCurrency,
		}
		if ctx.ParticipantEmail != nil {
			out.ParticipantEmail = *ctx.ParticipantEmail
		}
		_ = audit.Record(r.Context(), s.pool, audit.Entry{
			ActorUserID: &user.ID,
			EventID:     &eventID,
			Action:      "ticket.check_in_manual",
			TargetKind:  audit.TargetTicket,
			TargetID:    ticketID.String(),
			Payload: map[string]any{
				"outcome":           outcome,
				"participant":       ctx.ParticipantName,
				"paid_amount_minor": updated.PaidAmountMinor,
			},
		})
		logger.Info("manual check-in", "outcome", outcome, "ticket_id", ticketID, "amount", updated.PaidAmountMinor)
		writeJSON(w, http.StatusOK, out)
		return
	}

	// Phase 1: lookup by free-text query → return candidate participants.
	// Accepts substring matches across participant name, participant email,
	// booking contact name + email, AND the booking-reference prefix. Returns
	// one row per *participant* (= per ticket) so the staffer taps a single
	// person to check them in — not a whole booking at once.
	if req.Reference == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "reference, query, or ticket_id required")
		return
	}
	queryStr := strings.TrimSpace(req.Reference)
	if len(queryStr) < 2 {
		writeErr(w, http.StatusBadRequest, "query_too_short", "type at least 2 characters")
		return
	}
	rows, err := q.SearchActiveTicketsForEvent(r.Context(), db.SearchActiveTicketsForEventParams{
		EventID: eventID,
		Column2: queryStr,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]manualTicketRow, 0, len(rows))
	for _, t := range rows {
		row := manualTicketRow{
			ID:               t.ID,
			ParticipantName:  t.ParticipantName,
			BookingReference: formatReference(t.BookingID),
			CheckedInAt:      t.CheckedInAt,
		}
		if t.ParticipantEmail != nil {
			row.ParticipantEmail = *t.ParticipantEmail
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": out})
}

// HandleUndoCheckIn: POST /api/admin/events/{id}/scan/undo
//
// Reverses an accidental check-in. Body: {"ticket_id": "<uuid>"}. Clears
// checked_in_at + paid_amount_minor and flips status back to 'paid'.
// Idempotent: undoing a not-checked-in ticket returns 200 ok with the
// `already_undone` flag (no error — the staffer may have hit it twice in a
// rushed door situation).
type undoCheckInRequest struct {
	TicketID string `json:"ticket_id"`
}

func (s *Service) HandleUndoCheckIn(w http.ResponseWriter, r *http.Request) {
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
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingCheckIn, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var req undoCheckInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	ticketID, err := uuid.Parse(strings.TrimSpace(req.TicketID))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_ticket_id", err.Error())
		return
	}
	q := s.pool.Queries()
	row, err := q.GetTicketByID(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "ticket not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row.EventID != eventID {
		writeErr(w, http.StatusConflict, "wrong_event", "")
		return
	}
	if row.Status != "checked_in" {
		// Idempotent — caller may have double-tapped. Don't error.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "already_undone": true})
		return
	}
	priorAmount := row.PaidAmountMinor
	priorAt := row.CheckedInAt
	if _, err := q.UndoCheckInTicket(r.Context(), ticketID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		EventID:     &eventID,
		Action:      "ticket.check_in_undo",
		TargetKind:  audit.TargetTicket,
		TargetID:    ticketID.String(),
		Payload: map[string]any{
			"prior_amount_minor":  priorAmount,
			"prior_checked_in_at": priorAt,
		},
	})
	logger.Info("check-in undo", "ticket_id", ticketID, "prior_amount", priorAmount)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "already_undone": false})
}

// HandleCheckInStatus: GET /api/admin/events/{id}/check-ins
//
// Powers the scanner page header (live X von Y) and the recent-activity
// log. Defaults to 10 recent rows; ?limit=N caps at 50.
func (s *Service) HandleCheckInStatus(w http.ResponseWriter, r *http.Request) {
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
	if err := authz.Allow(r.Context(), s.pool.Queries(), user, authz.ActionBookingCheckIn, authz.ForEvent(eventID)); err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	limit := int32(10)
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int32
		_, _ = fmtSscanInt(v, &n)
		if n > 0 && n <= 50 {
			limit = n
		}
	}
	q := s.pool.Queries()
	counts, err := q.CountCheckedInForEvent(r.Context(), eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	rows, err := q.ListRecentCheckInsForEvent(r.Context(), db.ListRecentCheckInsForEventParams{
		EventID: eventID,
		Limit:   limit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	recent := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		entry := map[string]any{
			"id":                r.ID,
			"participant_name":  r.ParticipantName,
			"booking_reference": r.BookingReference,
		}
		if r.CheckedInAt != nil {
			entry["checked_in_at"] = r.CheckedInAt.Format(time.RFC3339)
		}
		if r.ParticipantEmail != nil {
			entry["participant_email"] = *r.ParticipantEmail
		}
		if r.PaidAmountMinor != nil {
			entry["paid_amount_minor"] = *r.PaidAmountMinor
		}
		recent = append(recent, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"expected":   int(counts.Expected),
		"checked_in": int(counts.CheckedIn),
		"recent":     recent,
	})
}

// --- helpers ----------------------------------------------------------------

func (s *Service) auditScanFailure(r *http.Request, actorID uuid.UUID, eventID uuid.UUID, code, token, dev string) {
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actorID,
		EventID:     &eventID,
		Action:      "ticket.check_in_failed",
		TargetKind:  audit.TargetEvent,
		TargetID:    eventID.String(),
		Payload: map[string]any{
			"reason":  code,
			"message": dev,
			// Tokens are sensitive — log only a short hash-friendly prefix.
			"token_prefix": safePrefix(token, 12),
		},
	})
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func stripNonHex(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fmtSscanInt is a tiny stand-in for fmt.Sscanf("%d") to keep the file from
// pulling in fmt just for one int parse.
func fmtSscanInt(s string, out *int32) (int, error) {
	var n int32
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + (c - '0')
		if n > 1_000_000 {
			break
		}
	}
	*out = n
	return 1, nil
}
