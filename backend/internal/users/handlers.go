package users

import (
	"crypto/rand"
	"encoding/base64"
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
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
)

// Helpers --------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, devMsg string) {
	writeJSON(w, status, map[string]any{"error": code, "developer_message": devMsg})
}

// userDTO ----------------------------------------------------------------

type userDTO struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Role          string     `json:"role"`
	DisplayName   string     `json:"display_name"`
	Active        bool       `json:"active"`
	Disabled      bool       `json:"disabled"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	SessionsCount int        `json:"sessions_count"`
}

func toDTO(u db.ListUsersRow, sessions int) userDTO {
	d := userDTO{
		ID:            u.ID,
		Email:         u.Email,
		Role:          u.Role,
		Active:        u.Active,
		Disabled:      u.DisabledAt != nil,
		CreatedAt:     u.CreatedAt,
		LastLoginAt:   u.LastLoginAt,
		SessionsCount: sessions,
	}
	if u.DisplayName != nil {
		d.DisplayName = *u.DisplayName
	}
	return d
}

// HandleList — GET /api/admin/users (global_admin only) ----------------------

func (s *Service) HandleList(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	rows, err := s.pool.Queries().ListUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]userDTO, 0, len(rows))
	for _, u := range rows {
		// Best-effort session count per row. Cheap because sessions table is small.
		n, _ := s.pool.Queries().CountSessionsForUser(r.Context(), u.ID)
		out = append(out, toDTO(u, int(n)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// HandleInvite — POST /api/admin/users -------------------------------------

type inviteRequest struct {
	Email       string `json:"email"`
	Role        string `json:"role"` // global_admin | event_admin | event_manager
	DisplayName string `json:"display_name"`
}

func (s *Service) HandleInvite(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())
	actor, ok := auth.UserFromContext(r.Context())
	if !ok || actor.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}

	var req inviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Role = strings.TrimSpace(req.Role)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if !strings.Contains(req.Email, "@") {
		writeErr(w, http.StatusBadRequest, "invalid_email", "email required")
		return
	}
	role := domain.Role(req.Role)
	if !role.Valid() {
		writeErr(w, http.StatusBadRequest, "invalid_role", "role must be global_admin, event_admin, or event_manager")
		return
	}

	// Already-existing email? Return 409 — re-inviting is a separate flow
	// (not in MVP; admin can deactivate then reinvite if really needed).
	if _, err := s.pool.Queries().GetUserByEmail(r.Context(), req.Email); err == nil {
		writeErr(w, http.StatusConflict, "user_exists", "a user with that email already exists")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Placeholder password hash — random bytes argon2id-hashed. Will be
	// overwritten on setup completion. Required because users.password_hash
	// is NOT NULL.
	placeholderRaw := make([]byte, 32)
	if _, err := rand.Read(placeholderRaw); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	placeholderHash, err := auth.HashPassword(base64.RawStdEncoding.EncodeToString(placeholderRaw))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	var displayNamePtr *string
	if req.DisplayName != "" {
		displayNamePtr = &req.DisplayName
	}

	var created db.User
	var rawToken string
	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		row, err := tx.CreateInvitedUser(r.Context(), db.CreateInvitedUserParams{
			Email:        req.Email,
			PasswordHash: placeholderHash,
			Role:         string(role),
			DisplayName:  displayNamePtr,
		})
		if err != nil {
			return err
		}
		created = row

		raw, hash, err := newToken()
		if err != nil {
			return err
		}
		rawToken = raw

		_, err = tx.CreateSetupToken(r.Context(), db.CreateSetupTokenParams{
			UserID:    row.ID,
			TokenHash: hash,
			ExpiresAt: time.Now().Add(s.cfg.SetupTokenTTL),
		})
		return err
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      "user.invite",
		TargetKind:  audit.TargetUser,
		TargetID:    created.ID.String(),
		Payload:     map[string]any{"email": created.Email, "role": created.Role},
	})

	if err := s.sendInviteEmail(r.Context(), created, rawToken); err != nil {
		logger.Warn("invite email send failed", "err", err, "user_id", created.ID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    created.ID,
		"email": created.Email,
		"role":  created.Role,
	})
}

// HandleResendInvite — POST /api/admin/users/{id}/resend-invite -------------
//
// Invalidates any outstanding setup token for the user, mints a new one, and
// re-sends the invite email. Only valid for users who were invited but never
// completed setup (active=false, not disabled). For already-active users the
// admin should use set-password instead.
func (s *Service) HandleResendInvite(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())
	actor, ok := auth.UserFromContext(r.Context())
	if !ok || actor.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	target, err := s.pool.Queries().GetUserByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if target.DisabledAt != nil {
		writeErr(w, http.StatusBadRequest, "user_disabled", "reactivate the user before resending the invite")
		return
	}
	if target.Active {
		writeErr(w, http.StatusBadRequest, "user_already_active", "user has already completed setup; use set-password instead")
		return
	}

	var rawToken string
	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		if err := tx.InvalidateOutstandingSetupTokensForUser(r.Context(), target.ID); err != nil {
			return err
		}
		raw, hash, err := newToken()
		if err != nil {
			return err
		}
		rawToken = raw
		_, err = tx.CreateSetupToken(r.Context(), db.CreateSetupTokenParams{
			UserID:    target.ID,
			TokenHash: hash,
			ExpiresAt: time.Now().Add(s.cfg.SetupTokenTTL),
		})
		return err
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      "user.invite_resend",
		TargetKind:  audit.TargetUser,
		TargetID:    target.ID.String(),
		Payload:     map[string]any{"email": target.Email},
	})

	if err := s.sendInviteEmail(r.Context(), target, rawToken); err != nil {
		logger.Warn("invite email resend failed", "err", err, "user_id", target.ID)
		writeErr(w, http.StatusBadGateway, "mail_send_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleDeactivate — POST /api/admin/users/{id}/deactivate -------------------

func (s *Service) HandleDeactivate(w http.ResponseWriter, r *http.Request) {
	s.toggleDisabled(w, r, true)
}

// HandleReactivate — POST /api/admin/users/{id}/reactivate -------------------

func (s *Service) HandleReactivate(w http.ResponseWriter, r *http.Request) {
	s.toggleDisabled(w, r, false)
}

func (s *Service) toggleDisabled(w http.ResponseWriter, r *http.Request, disable bool) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok || actor.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	if id == actor.ID && disable {
		writeErr(w, http.StatusBadRequest, "cannot_self_deactivate", "you cannot deactivate yourself")
		return
	}
	q := s.pool.Queries()
	if disable {
		if err := q.DisableUser(r.Context(), id); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		// Force-logout: bump password_version → all sessions invalidated.
		_ = q.BumpPasswordVersion(r.Context(), id)
	} else {
		if err := q.ReactivateUser(r.Context(), id); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	action := "user.deactivate"
	if !disable {
		action = "user.reactivate"
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      action,
		TargetKind:  audit.TargetUser,
		TargetID:    id.String(),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleRoleChange — POST /api/admin/users/{id}/role ------------------------

type roleChangeRequest struct {
	Role string `json:"role"`
}

func (s *Service) HandleRoleChange(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok || actor.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	var req roleChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	role := domain.Role(strings.TrimSpace(req.Role))
	if !role.Valid() {
		writeErr(w, http.StatusBadRequest, "invalid_role", "role must be global_admin, event_admin, or event_manager")
		return
	}
	// Prevent demoting the last global_admin — keeps somebody able to manage.
	target, err := s.pool.Queries().GetUserByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if domain.Role(target.Role) == domain.RoleGlobalAdmin && role != domain.RoleGlobalAdmin {
		n, _ := s.pool.Queries().CountGlobalAdmins(r.Context())
		if int(n) <= 1 {
			writeErr(w, http.StatusBadRequest, "last_global_admin", "cannot demote the only global_admin")
			return
		}
	}
	if err := s.pool.Queries().SetUserRole(r.Context(), db.SetUserRoleParams{ID: id, Role: string(role)}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      "user.role_change",
		TargetKind:  audit.TargetUser,
		TargetID:    id.String(),
		Payload:     map[string]any{"new_role": role, "old_role": target.Role},
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleSetupComplete — POST /setup/{token} (public) -------------------------

type setupRequest struct {
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
	// Optional: invitee picks their display name on setup. Empty / whitespace
	// means "keep what the inviter set" — typically that's empty too, so the
	// admin user list will fall back to email.
	DisplayName string `json:"display_name,omitempty"`
}

func (s *Service) HandleSetupComplete(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())
	tok := chi.URLParam(r, "token")
	if tok == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "token required")
		return
	}
	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Password != req.PasswordConfirm {
		writeErr(w, http.StatusBadRequest, "password_mismatch", "passwords do not match")
		return
	}
	if len(req.Password) < s.cfg.MinPasswordChars {
		writeErr(w, http.StatusBadRequest, "password_too_short", "password must be at least 12 characters")
		return
	}

	row, err := s.pool.Queries().GetSetupTokenByHash(r.Context(), hashToken(tok))
	if err != nil {
		// Hide whether the token never existed vs. expired vs. used — single
		// "invalid or expired" surface so attackers can't enumerate.
		writeErr(w, http.StatusUnauthorized, "invalid_token", "token invalid or expired")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		if err := tx.SetUserPasswordAndActivate(r.Context(), db.SetUserPasswordAndActivateParams{
			ID:           row.UserID,
			PasswordHash: hash,
		}); err != nil {
			return err
		}
		// If the invitee supplied a display name, set it. We treat an
		// explicit empty string as "no change" — admins might pre-fill the
		// display name in the invite form and we don't want to wipe that.
		if dn := strings.TrimSpace(req.DisplayName); dn != "" {
			ptr := &dn
			if err := tx.UpdateUserDisplayName(r.Context(), db.UpdateUserDisplayNameParams{
				ID:          row.UserID,
				DisplayName: ptr,
			}); err != nil {
				return err
			}
		}
		return tx.ConsumeSetupToken(r.Context(), row.ID)
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &row.UserID,
		Action:      "user.setup_complete",
		TargetKind:  audit.TargetUser,
		TargetID:    row.UserID.String(),
	})
	logger.Info("user setup complete", "user_id", row.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleAccountChangePassword — POST /api/admin/account/password ------------

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Service) HandleAccountChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		return
	}
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(req.NewPassword) < s.cfg.MinPasswordChars {
		writeErr(w, http.StatusBadRequest, "password_too_short", "password must be at least 12 characters")
		return
	}

	row, err := s.pool.Queries().GetUserByID(r.Context(), user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	ok2, err := auth.VerifyPassword(req.CurrentPassword, row.PasswordHash)
	if err != nil || !ok2 {
		writeErr(w, http.StatusUnauthorized, "wrong_current_password", "current password incorrect")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.pool.Queries().UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{
		ID:           user.ID,
		PasswordHash: hash,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		Action:      "user.password_change",
		TargetKind:  audit.TargetUser,
		TargetID:    user.ID.String(),
	})
	// Sign current session out — UpdateUserPassword bumped password_version,
	// so any cookie still pointing at this session is invalid by next request.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "all_sessions_signed_out": true})
}

// HandleForceLogout — POST /api/admin/users/{id}/force-logout ----------------
//
// Bumps the user's password_version → all sessions invalidated. No password
// change required (admin operation). Self-target allowed (single device kick).
func (s *Service) HandleForceLogout(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok || actor.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	if err := s.pool.Queries().BumpPasswordVersion(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      "user.force_logout",
		TargetKind:  audit.TargetUser,
		TargetID:    id.String(),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
