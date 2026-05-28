package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/dsteiman/tickets-general/backend/internal/logging"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	// Populated only when the current session was minted via
	// /admin/users/{id}/impersonate; otherwise empty. The frontend uses
	// this to render the "Du arbeitest gerade als …" banner and the
	// "Zurück zu mir" button.
	ImpersonatorEmail string `json:"impersonator_email,omitempty"`
}

// HandleLogin processes POST /api/auth/login. Body: {email, password}.
func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "email and password required")
		return
	}

	sw, err := s.Login(r.Context(), w, r, req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUserDisabled):
			writeJSONError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		default:
			logging.FromContext(r.Context()).Error("login failed", "err", err, slog.String("event", "login.error"))
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, struct {
		User userResponse `json:"user"`
	}{
		User: toUserResponse(sw.User.ID.String(), sw.User.Email, string(sw.User.Role), sw.User.DisplayName),
	})
}

// HandleLogout processes POST /api/auth/logout. Idempotent: clears cookie regardless.
func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.Logout(r.Context(), w, r); err != nil {
		logging.FromContext(r.Context()).Warn("logout cleanup failed", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleMe returns the authenticated user. Mounted behind Middleware(true).
func (s *Service) HandleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	resp := toUserResponse(u.ID.String(), u.Email, string(u.Role), u.DisplayName)
	if imp, ok := ImpersonatorFromContext(r.Context()); ok {
		resp.ImpersonatorEmail = imp.Email
	}
	writeJSON(w, http.StatusOK, resp)
}

func toUserResponse(id, email, role, displayName string) userResponse {
	return userResponse{ID: id, Email: email, Role: role, DisplayName: displayName}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeJSONError emits the canonical error shape for this API:
//
//	{"error": "<stable_code>", "developer_message": "<for-debugging-only>"}
//
// Frontends look up the user-facing display text by `error` in their own
// message catalog. `developer_message` is for operators/logs and may change.
func writeJSONError(w http.ResponseWriter, status int, code, developerMessage string) {
	writeJSON(w, status, map[string]any{
		"error":             code,
		"developer_message": developerMessage,
	})
}
