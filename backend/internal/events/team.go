package events

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/audit"
	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

type teamUserDTO struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
}

type teamResponse struct {
	Admins   []teamUserDTO `json:"admins"`
	Managers []teamUserDTO `json:"managers"`
}

// HandleListTeam: GET /api/admin/events/{id}/team
func (s *Service) HandleListTeam(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}

	q := s.pool.Queries()
	adminRows, err := q.ListEventAdminUsers(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	managerRows, err := q.ListEventManagerUsers(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := teamResponse{
		Admins:   make([]teamUserDTO, 0, len(adminRows)),
		Managers: make([]teamUserDTO, 0, len(managerRows)),
	}
	for _, a := range adminRows {
		dn := ""
		if a.DisplayName != nil {
			dn = *a.DisplayName
		}
		resp.Admins = append(resp.Admins, teamUserDTO{ID: a.ID, Email: a.Email, DisplayName: dn})
	}
	for _, m := range managerRows {
		dn := ""
		if m.DisplayName != nil {
			dn = *m.DisplayName
		}
		resp.Managers = append(resp.Managers, teamUserDTO{ID: m.ID, Email: m.Email, DisplayName: dn})
	}
	writeJSON(w, http.StatusOK, resp)
}

type assignRequest struct {
	UserID uuid.UUID `json:"user_id"`
}

// HandleAssignAdmin: POST /api/admin/events/{id}/admins
func (s *Service) HandleAssignAdmin(w http.ResponseWriter, r *http.Request) {
	s.assign(w, r, true)
}

// HandleUnassignAdmin: DELETE /api/admin/events/{id}/admins/{userId}
func (s *Service) HandleUnassignAdmin(w http.ResponseWriter, r *http.Request) {
	s.unassign(w, r, true)
}

// HandleAssignManager: POST /api/admin/events/{id}/managers
func (s *Service) HandleAssignManager(w http.ResponseWriter, r *http.Request) {
	s.assign(w, r, false)
}

// HandleUnassignManager: DELETE /api/admin/events/{id}/managers/{userId}
func (s *Service) HandleUnassignManager(w http.ResponseWriter, r *http.Request) {
	s.unassign(w, r, false)
}

func (s *Service) assign(w http.ResponseWriter, r *http.Request, asAdmin bool) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	var req assignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	target, err := s.pool.Queries().GetUserByID(r.Context(), req.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "user_not_found", "target user does not exist")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if target.DisabledAt != nil {
		writeErr(w, http.StatusBadRequest, "user_disabled", "target user is disabled")
		return
	}
	if asAdmin {
		// Only event_admin or global_admin base role can sit in the event_admins table.
		if domain.Role(target.Role) != domain.RoleEventAdmin && domain.Role(target.Role) != domain.RoleGlobalAdmin {
			writeErr(w, http.StatusBadRequest, "wrong_role", "target user is not an event_admin")
			return
		}
		if err := s.pool.Queries().AssignEventAdmin(r.Context(), db.AssignEventAdminParams{UserID: target.ID, EventID: eventID}); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		s.recordEventAudit(r.Context(), actor.ID, eventID, audit.ActionEventAddAdmin, map[string]any{"user_id": target.ID.String()})
	} else {
		if domain.Role(target.Role) != domain.RoleEventManager && domain.Role(target.Role) != domain.RoleEventAdmin && domain.Role(target.Role) != domain.RoleGlobalAdmin {
			writeErr(w, http.StatusBadRequest, "wrong_role", "target user cannot be a manager")
			return
		}
		if err := s.pool.Queries().AssignEventManager(r.Context(), db.AssignEventManagerParams{UserID: target.ID, EventID: eventID}); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		s.recordEventAudit(r.Context(), actor.ID, eventID, audit.ActionEventAddManager, map[string]any{"user_id": target.ID.String()})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) unassign(w http.ResponseWriter, r *http.Request, asAdmin bool) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_user_id", "must be a UUID")
		return
	}
	actor, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	q := s.pool.Queries()
	if asAdmin {
		if err := q.UnassignEventAdmin(r.Context(), db.UnassignEventAdminParams{UserID: userID, EventID: eventID}); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else {
		if err := q.UnassignEventManager(r.Context(), db.UnassignEventManagerParams{UserID: userID, EventID: eventID}); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	s.recordEventAudit(r.Context(), actor.ID, eventID, "event.remove_team_member", map[string]any{
		"user_id":  userID.String(),
		"as_admin": asAdmin,
	})
	w.WriteHeader(http.StatusNoContent)
}
