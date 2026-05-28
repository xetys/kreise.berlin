package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

// HandleImpersonate: POST /api/admin/users/{id}/impersonate
//
// Issues a fresh session for the target user with `impersonator_user_id` set
// to the calling global_admin. Cookies are swapped so the rest of the
// request lifecycle behaves as the target. The original global_admin's
// session is NOT touched — they can "stop impersonating" later, which mints
// a fresh session for them and revokes the impersonation session.
//
// Refuses to:
//   - impersonate yourself (no-op anyway)
//   - impersonate another global_admin (no privilege escalation possible
//     here, but it's a weird operation that suggests confusion; explicit 400)
//   - run while *already* impersonating (one level deep only; the admin
//     should stop first)
func (s *Service) HandleImpersonate(w http.ResponseWriter, r *http.Request) {
	actor, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	if actor.Role != domain.RoleGlobalAdmin {
		writeJSONError(w, http.StatusForbidden, "forbidden", "global_admin required")
		return
	}
	if _, alreadyImp := ImpersonatorFromContext(r.Context()); alreadyImp {
		writeJSONError(w, http.StatusConflict, "already_impersonating", "stop impersonating first")
		return
	}

	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "must be a UUID")
		return
	}
	if targetID == actor.ID {
		writeJSONError(w, http.StatusBadRequest, "cannot_impersonate_self", "")
		return
	}

	target, err := s.pool.Queries().GetUserByID(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "not_found", "")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if target.DisabledAt != nil || !target.Active {
		writeJSONError(w, http.StatusBadRequest, "user_inactive", "cannot impersonate inactive user")
		return
	}
	if domain.Role(target.Role) == domain.RoleGlobalAdmin {
		writeJSONError(w, http.StatusBadRequest, "target_is_global_admin", "global_admins already have unrestricted access")
		return
	}

	sessionID, err := newSessionID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "session id: "+err.Error())
		return
	}
	ua := truncate(r.UserAgent(), 500)
	ip := remoteAddrIP(r)
	actorID := actor.ID
	created, err := s.pool.Queries().CreateSession(r.Context(), db.CreateSessionParams{
		ID:                 sessionID,
		UserID:             target.ID,
		ExpiresAt:          time.Now().Add(s.cfg.SessionTTL),
		UserAgent:          nilIfEmpty(ua),
		Ip:                 ip,
		PasswordVersion:    target.PasswordVersion,
		ImpersonatorUserID: &actorID,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "create session: "+err.Error())
		return
	}

	// Revoke the *current* (admin's own) session so they can't continue acting
	// as themselves in another tab while impersonating in this one. They'll
	// get a fresh session on stop-impersonating.
	if currentID, err := s.readSessionID(r); err == nil {
		_ = s.pool.Queries().RevokeSession(r.Context(), currentID)
	}

	s.setCookie(w, sessionID, created.ExpiresAt)

	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &actor.ID,
		Action:      "user.impersonate_start",
		TargetKind:  audit.TargetUser,
		TargetID:    target.ID.String(),
		Payload:     map[string]any{"target_email": target.Email, "target_role": target.Role},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"impersonating": target.Email,
		"impersonator":  actor.Email,
	})
}

// HandleStopImpersonating: POST /api/auth/stop-impersonating
//
// Revokes the current impersonation session and mints a fresh one for the
// original impersonator. Idempotent — calling it without an active
// impersonation just signs you out (returns to the login page on the
// frontend).
func (s *Service) HandleStopImpersonating(w http.ResponseWriter, r *http.Request) {
	imp, ok := ImpersonatorFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "not_impersonating", "")
		return
	}
	impID, err := uuid.Parse(imp.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "bad impersonator id: "+err.Error())
		return
	}

	// Revoke the impersonation session.
	if currentID, err := s.readSessionID(r); err == nil {
		_ = s.pool.Queries().RevokeSession(r.Context(), currentID)
	}

	// Mint a fresh session for the original global_admin.
	original, err := s.pool.Queries().GetUserByID(r.Context(), impID)
	if err != nil {
		// The original admin was deleted — clear the cookie and ask them to log back in.
		s.clearCookie(w)
		writeJSONError(w, http.StatusGone, "impersonator_gone", "original admin no longer exists; sign in again")
		return
	}
	if original.DisabledAt != nil || !original.Active {
		s.clearCookie(w)
		writeJSONError(w, http.StatusForbidden, "impersonator_inactive", "original admin is no longer active; sign in again")
		return
	}

	newID, err := newSessionID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "session id: "+err.Error())
		return
	}
	created, err := s.pool.Queries().CreateSession(r.Context(), db.CreateSessionParams{
		ID:                 newID,
		UserID:             original.ID,
		ExpiresAt:          time.Now().Add(s.cfg.SessionTTL),
		UserAgent:          nilIfEmpty(truncate(r.UserAgent(), 500)),
		Ip:                 remoteAddrIP(r),
		PasswordVersion:    original.PasswordVersion,
		ImpersonatorUserID: nil,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "create session: "+err.Error())
		return
	}
	s.setCookie(w, newID, created.ExpiresAt)

	_ = audit.Record(r.Context(), s.pool, audit.Entry{
		ActorUserID: &original.ID,
		Action:      "user.impersonate_stop",
		TargetKind:  audit.TargetUser,
		TargetID:    "",
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restored_to": original.Email})
}

// _ avoids unused-import warnings during partial builds.
var _ context.Context
var _ = fmt.Sprintf
