package tickets

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// MountPublic mounts the public ticket-page surface. All endpoints are gated
// by the signed view token in the URL parameter — possession of a valid
// view token grants access to that one ticket. No auth session.
func MountPublic(r chi.Router, s *Service) {
	r.Route("/tickets/{token}", func(r chi.Router) {
		r.Get("/", s.HandleGet)
		r.Get("/qr.png", s.HandleQR)
		r.Post("/cancel", s.HandleCancel)
		r.Post("/transfer", s.HandleTransfer)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, devMsg string) {
	writeJSON(w, status, map[string]any{"error": code, "developer_message": devMsg})
}

// writeTokenErr translates verification + ticket-resolution errors to the
// canonical API error shape.
func writeTokenErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tokens.ErrInvalid),
		errors.Is(err, tokens.ErrBadSignature),
		errors.Is(err, tokens.ErrBadPurpose):
		writeErr(w, http.StatusUnauthorized, "invalid_token", err.Error())
	case errors.Is(err, errTicketNotFound):
		writeErr(w, http.StatusNotFound, "ticket_not_found", "no ticket for that token")
	case errors.Is(err, errStaleToken):
		// Token decoded fine but the nonce on the ticket has rotated since
		// (transfer happened, token from a previous holder).
		writeErr(w, http.StatusGone, "stale_token", "this link has been superseded by a transfer")
	default:
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
