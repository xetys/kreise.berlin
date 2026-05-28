package tickets

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/skip2/go-qrcode"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// ============================================================================
// View / GET
// ============================================================================

type ticketResponse struct {
	TicketID       string    `json:"ticket_id"`
	Status         string    `json:"status"`
	HolderName     string    `json:"holder_name"`
	HolderEmail    string    `json:"holder_email,omitempty"`
	AmountMinor    int64     `json:"amount_minor"`
	Currency       string    `json:"currency"`
	PhaseName      string    `json:"phase_name,omitempty"`
	CategoryName   string    `json:"category_name,omitempty"`
	DurationName   string    `json:"duration_name,omitempty"`
	HasQR          bool      `json:"has_qr"`
	QRURL          string    `json:"qr_url,omitempty"`
	CanCancel      bool      `json:"can_cancel"`
	CanTransfer    bool      `json:"can_transfer"`
	Event          eventInfo `json:"event"`
	BookingContact string    `json:"booking_contact"`
	CheckedInAt    string    `json:"checked_in_at,omitempty"`
	CanceledAt     string    `json:"canceled_at,omitempty"`
}

type eventInfo struct {
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	Location       string    `json:"location"`
	ColorPrimary   string    `json:"color_primary"`
	ColorSecondary string    `json:"color_secondary"`
	ColorText      string    `json:"color_text"`
}

// HandleGet: GET /api/tickets/{viewToken}
func (s *Service) HandleGet(w http.ResponseWriter, r *http.Request) {
	row, err := s.resolve(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeTokenErr(w, err)
		return
	}

	resp := ticketResponse{
		TicketID:       row.ID.String(),
		Status:         row.Status,
		HolderName:     row.ParticipantName,
		AmountMinor:    row.AmountMinor,
		Currency:       row.EventCurrency,
		BookingContact: row.BookingContactEmail,
		Event: eventInfo{
			Name:           row.EventName,
			Slug:           row.EventSlug,
			StartsAt:       row.EventStartsAt,
			EndsAt:         row.EventEndsAt,
			Location:       row.EventLocation,
			ColorPrimary:   row.EventColorPrimary,
			ColorSecondary: row.EventColorSecondary,
			ColorText:      row.EventColorText,
		},
		CanCancel:   canCancel(row.Status),
		CanTransfer: canTransfer(row.Status),
		HasQR:       row.Status == "paid" || row.Status == "checked_in",
	}
	if row.ParticipantEmail != nil {
		resp.HolderEmail = *row.ParticipantEmail
	}
	if row.PhaseName != nil {
		resp.PhaseName = *row.PhaseName
	}
	if row.CategoryName != nil {
		resp.CategoryName = *row.CategoryName
	}
	if row.DurationName != nil {
		resp.DurationName = *row.DurationName
	}
	if resp.HasQR {
		resp.QRURL = "/api/tickets/" + chi.URLParam(r, "token") + "/qr.png"
	}
	if row.CheckedInAt != nil {
		resp.CheckedInAt = row.CheckedInAt.Format(time.RFC3339)
	}
	if row.CanceledAt != nil {
		resp.CanceledAt = row.CanceledAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, resp)
}

// ============================================================================
// QR PNG
// ============================================================================

// HandleQR: GET /api/tickets/{viewToken}/qr.png
//
// Encodes the ticket's qr_token (signed with PurposeQR — different from the
// view token) into a PNG. Door scanning (Phase 8) decodes that string and
// posts it to the check-in endpoint.
func (s *Service) HandleQR(w http.ResponseWriter, r *http.Request) {
	row, err := s.resolve(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeTokenErr(w, err)
		return
	}
	if row.Status != "paid" && row.Status != "checked_in" {
		writeErr(w, http.StatusForbidden, "qr_unavailable", "QR is only available for paid tickets")
		return
	}

	png, err := qrcode.Encode(row.QrToken, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "qr_render_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300") // ticket holders re-open often
	_, _ = w.Write(png)
}

// ============================================================================
// Cancel
// ============================================================================

// HandleCancel: POST /api/tickets/{viewToken}/cancel
func (s *Service) HandleCancel(w http.ResponseWriter, r *http.Request) {
	row, err := s.resolve(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeTokenErr(w, err)
		return
	}
	if !canCancel(row.Status) {
		// idempotent: already canceled is fine; checked_in is not.
		if row.Status == "canceled" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeErr(w, http.StatusConflict, "cannot_cancel", "ticket cannot be canceled in state "+row.Status)
		return
	}

	if err := s.pool.Queries().CancelTicketByID(r.Context(), row.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Audit (actor unknown — public ticket-page action).
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		EventID:    &row.EventID,
		Action:     "ticket.cancel",
		TargetKind: audit.TargetTicket,
		TargetID:   row.ID.String(),
		Payload:    map[string]any{"holder_name": row.ParticipantName},
	})

	// Waitlist rotation: a per-ticket cancel just freed a seat. Fire the
	// promote hook (no-op when not wired or when the event is unlimited).
	if s.onCancel != nil {
		eventID := row.EventID
		go s.onCancel(context.Background(), eventID, slog.Default())
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Transfer
// ============================================================================

type transferRequest struct {
	NewName  string `json:"new_name"`
	NewEmail string `json:"new_email"`
}

type transferResponse struct {
	NewViewToken string `json:"new_view_token"`
	NewViewURL   string `json:"new_view_url,omitempty"`
}

// HandleTransfer: POST /api/tickets/{viewToken}/transfer
//
// Atomically: updates the participant row (name + email) and rotates the
// ticket's qr_nonce + qr_token. Old QR + view tokens become invalid. Sends
// a fresh confirmation-style email to the new holder if they have an email.
func (s *Service) HandleTransfer(w http.ResponseWriter, r *http.Request) {
	row, err := s.resolve(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeTokenErr(w, err)
		return
	}
	if !canTransfer(row.Status) {
		writeErr(w, http.StatusConflict, "cannot_transfer", "ticket cannot be transferred in state "+row.Status)
		return
	}

	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.NewName == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "new_name required")
		return
	}

	// Generate a fresh nonce + sign the new qr_token.
	newNonce := make([]byte, 16)
	if _, err := rand.Read(newNonce); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	newQR, err := tokens.Sign(tokens.PurposeQR, row.ID, newNonce, s.cfg.TokenSigningKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		var emailPtr *string
		if req.NewEmail != "" {
			e := req.NewEmail
			emailPtr = &e
		}
		if err := tx.UpdateParticipantContact(r.Context(), db.UpdateParticipantContactParams{
			ID:    row.ParticipantID,
			Name:  req.NewName,
			Email: emailPtr,
		}); err != nil {
			return fmt.Errorf("update participant: %w", err)
		}
		if err := tx.TransferTicket(r.Context(), db.TransferTicketParams{
			ID:      row.ID,
			QrNonce: newNonce,
			QrToken: newQR,
		}); err != nil {
			return fmt.Errorf("rotate ticket: %w", err)
		}
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		EventID:    &row.EventID,
		Action:     "ticket.transfer",
		TargetKind: audit.TargetTicket,
		TargetID:   row.ID.String(),
		Payload: map[string]any{
			"from_name": row.ParticipantName,
			"to_name":   req.NewName,
			"to_email":  req.NewEmail,
		},
	})

	// New view token for the new holder.
	newViewToken, err := tokens.Sign(tokens.PurposeView, row.ID, newNonce, s.cfg.TokenSigningKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := transferResponse{NewViewToken: newViewToken}
	if s.cfg.PublicBaseURL != "" {
		resp.NewViewURL = s.cfg.PublicBaseURL + "/de/tickets/" + newViewToken
	}

	// Phase 5+ could re-send a confirmation email here. Skipped for MVP —
	// the response body returns the new view URL so the UI shows it
	// immediately to the previous holder.
	logging.FromContext(r.Context()).Info("ticket transferred", "ticket_id", row.ID, "to", req.NewName)

	writeJSON(w, http.StatusOK, resp)
}

// ============================================================================
// Helpers
// ============================================================================

// resolve verifies the view token and loads the joined ticket row. Returns
// the same kind of context errors that writeTokenErr understands.
func (s *Service) resolve(ctx context.Context, viewToken string) (db.GetTicketWithContextRow, error) {
	id, nonce, err := tokens.Verify(tokens.PurposeView, viewToken, s.cfg.TokenSigningKey)
	if err != nil {
		return db.GetTicketWithContextRow{}, err
	}
	row, err := s.pool.Queries().GetTicketWithContext(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetTicketWithContextRow{}, errTicketNotFound
		}
		return db.GetTicketWithContextRow{}, err
	}
	// Constant-time-ish: nonce on the row must match the one in the token.
	if !bytesEqual(row.QrNonce, nonce) {
		return db.GetTicketWithContextRow{}, errStaleToken
	}
	return row, nil
}

var (
	errTicketNotFound = errors.New("ticket_not_found")
	errStaleToken     = errors.New("stale_token")
)

func canCancel(status string) bool { return status == "booked" || status == "paid" }
func canTransfer(status string) bool {
	return status == "booked" || status == "paid"
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// suppress unused import warning when not all branches reference uuid.
var _ = uuid.Nil
