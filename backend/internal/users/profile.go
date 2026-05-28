package users

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// ---------------------------------------------------------------------------
// HandleAccountUpdateProfile — PATCH /api/admin/account/profile
//
// Self-service profile edit for the currently-signed-in user. MVP surface
// is just display_name (the only mutable profile field on the users table).
// Email + role are read-only from this endpoint — email changes go through
// the (not-yet-built) verified-email flow, role changes are admin-only.
//
// Sending an empty string CLEARS the display name (the admin list then falls
// back to showing the email). Sending no field at all is a 400 to avoid
// silent no-ops.
// ---------------------------------------------------------------------------

type updateProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
}

func (s *Service) HandleAccountUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.DisplayName == nil {
		writeErr(w, http.StatusBadRequest, "no_changes", "display_name field is required")
		return
	}
	// Cap to a reasonable length; the DB has no constraint but we don't want
	// 10KB display names floating around the audit log.
	dn := strings.TrimSpace(*req.DisplayName)
	if len(dn) > 200 {
		writeErr(w, http.StatusBadRequest, "too_long", "display_name must be <= 200 characters")
		return
	}

	var dbDN *string
	if dn != "" {
		dbDN = &dn
	}
	if err := s.pool.Queries().UpdateUserDisplayName(r.Context(), db.UpdateUserDisplayNameParams{
		ID:          user.ID,
		DisplayName: dbDN,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		Action:      "user.profile_update",
		TargetKind:  audit.TargetUser,
		TargetID:    user.ID.String(),
		Payload:     map[string]any{"display_name": dn},
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "display_name": dn})
}
