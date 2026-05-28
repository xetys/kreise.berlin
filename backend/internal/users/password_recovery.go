package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/dsteiman/tickets-general/backend/internal/mail"
)

// ---------------------------------------------------------------------------
// HandleAdminSetPassword — POST /api/admin/users/{id}/set-password
//
// Admin override for cases where the invite token expired, the user lost their
// reset email, or they need credentials handed to them out-of-band (verified
// in-person, over a phone call, etc). global_admin only. Refuses self (use
// /api/admin/account/password instead — that one requires the current pw).
//
// Side effects:
//   - target.password_hash := argon2id(new)
//   - target.password_version bumped → all live sessions invalidated
//     (the target gets force-logged-out everywhere they were signed in)
//   - audit log captures actor + target + reason (optional body field)
//
// Does NOT send an email — the admin is presumed to be handing the password
// over themselves. If you want an email-based flow, use /forgot-password.
// ---------------------------------------------------------------------------

type adminSetPasswordRequest struct {
	NewPassword string `json:"new_password"`
	Reason      string `json:"reason,omitempty"` // free-text, ends up in audit
}

func (s *Service) HandleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	if actor.Role != domain.RoleGlobalAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	if targetID == actor.ID {
		writeErr(w, http.StatusBadRequest, "cannot_set_own_password",
			"use /api/admin/account/password for your own password")
		return
	}
	var req adminSetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(req.NewPassword) < s.cfg.MinPasswordChars {
		writeErr(w, http.StatusBadRequest, "password_too_short",
			fmt.Sprintf("password must be at least %d characters", s.cfg.MinPasswordChars))
		return
	}

	target, err := s.pool.Queries().GetUserByID(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.pool.Queries().UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{
		ID:           target.ID,
		PasswordHash: hash,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      "user.admin_set_password",
		TargetKind:  audit.TargetUser,
		TargetID:    target.ID.String(),
		Payload: map[string]any{
			"target_email": target.Email,
			"reason":       strings.TrimSpace(req.Reason),
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// HandleForgotPassword — POST /forgot-password  (public)
//
// Email-based password recovery. Always returns 200 regardless of whether the
// email matches a user — this prevents anyone (logged-in or not) from using
// the endpoint to enumerate which addresses have accounts.
//
// Rate limiting: the caller (router) wraps this with the standard per-IP
// limiter. There's no per-email throttle yet — that lives on the deferred
// hardening list. Tokens themselves are short-lived (cfg.ResetTokenTTL) so a
// flood of requests creates pile-up but no security issue.
// ---------------------------------------------------------------------------

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

func (s *Service) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req forgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		// Even a bad email returns 200 — keeps the response shape uniform so
		// timing/status can't be used as a side channel.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	user, err := s.pool.Queries().GetUserByEmail(r.Context(), email)
	if err != nil {
		// No such user → silently return 200. Log internally so legit operators
		// can still debug "I didn't get the email" claims.
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Info("forgot-password: unknown email", "email", email)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if user.DisabledAt != nil || !user.Active {
		// Disabled / never-set-up accounts shouldn't get a reset link — they
		// need re-invite or re-activation. Same 200 shape to keep the
		// observer in the dark about account status.
		logger.Info("forgot-password: inactive account", "email", email)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	raw, hash, err := newToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if _, err := s.pool.Queries().CreatePasswordResetToken(r.Context(), db.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(s.cfg.ResetTokenTTL),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if err := s.sendPasswordResetEmail(r.Context(), user, raw); err != nil {
		logger.Warn("password reset email failed", "err", err, "user_id", user.ID)
		// Don't surface — still 200 to the caller (the token is in the DB,
		// they could conceivably retry by triggering forgot-password again
		// which generates a fresh token).
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &user.ID,
		Action:      "user.password_reset_requested",
		TargetKind:  audit.TargetUser,
		TargetID:    user.ID.String(),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// HandleResetPassword — POST /reset-password/{token}  (public)
//
// Consumes a reset token, sets the new password, bumps password_version
// (kills any other sessions the user had), and signs the user in. Mirrors
// HandleSetupComplete almost exactly — same single "invalid or expired"
// surface to avoid enumeration.
// ---------------------------------------------------------------------------

func (s *Service) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())
	tok := chi.URLParam(r, "token")
	if tok == "" {
		writeErr(w, http.StatusBadRequest, "invalid_token", "token required")
		return
	}
	var req setupRequest // {password, password_confirm} — same shape
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Password != req.PasswordConfirm {
		writeErr(w, http.StatusBadRequest, "password_mismatch", "passwords do not match")
		return
	}
	if len(req.Password) < s.cfg.MinPasswordChars {
		writeErr(w, http.StatusBadRequest, "password_too_short",
			fmt.Sprintf("password must be at least %d characters", s.cfg.MinPasswordChars))
		return
	}

	row, err := s.pool.Queries().GetPasswordResetTokenByHash(r.Context(), hashToken(tok))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_token", "token invalid or expired")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	err = s.pool.WithTx(r.Context(), func(tx *db.Queries) error {
		if err := tx.UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{
			ID:           row.UserID,
			PasswordHash: hash,
		}); err != nil {
			return err
		}
		return tx.ConsumePasswordResetToken(r.Context(), row.ID)
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &row.UserID,
		Action:      "user.password_reset_completed",
		TargetKind:  audit.TargetUser,
		TargetID:    row.UserID.String(),
	})
	logger.Info("password reset completed", "user_id", row.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// sendPasswordResetEmail — bilingual reset-link mail.
// ---------------------------------------------------------------------------

func (s *Service) sendPasswordResetEmail(ctx context.Context, user db.User, rawToken string) error {
	if s.mailer == nil {
		return nil
	}
	url := s.buildResetURL(rawToken)
	displayName := user.Email
	if user.DisplayName != nil && *user.DisplayName != "" {
		displayName = *user.DisplayName
	}
	hours := int(s.cfg.ResetTokenTTL.Hours())
	if hours < 1 {
		hours = 1
	}

	bodyDE := fmt.Sprintf(
		"Hallo %s,\n\n"+
			"jemand (vermutlich du) hat ein neues Passwort für deinen kreise.berlin-Account angefordert. "+
			"Klicke auf den folgenden Link, um ein neues Passwort zu setzen:\n\n"+
			"%s\n\n"+
			"Der Link ist %d Stunde(n) gültig. Falls du das nicht warst, kannst du diese E-Mail einfach ignorieren — "+
			"dein aktuelles Passwort bleibt gültig.",
		displayName, url, hours,
	)
	bodyEN := fmt.Sprintf(
		"Hi %s,\n\n"+
			"someone (probably you) requested a new password for your kreise.berlin account. "+
			"Click the link below to set a new password:\n\n"+
			"%s\n\n"+
			"The link is valid for %d hour(s). If this wasn't you, just ignore this email — "+
			"your current password remains valid.",
		displayName, url, hours,
	)

	spec := mail.BilingualSpec{
		SubjectDE: "Passwort zurücksetzen · kreise.berlin",
		SubjectEN: "Reset your password · kreise.berlin",
		BodyDE:    bodyDE,
		BodyEN:    bodyEN,
	}
	msg, err := mail.RenderBilingualThemed("admin.password_reset", spec, mail.Theme{}, mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	msg.To = user.Email
	return s.mailer.Send(ctx, msg)
}
