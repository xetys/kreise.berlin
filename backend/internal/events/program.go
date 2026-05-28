package events

import (
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
)

type programEntryDTO struct {
	ID          uuid.UUID  `json:"id"`
	EventID     uuid.UUID  `json:"event_id"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Ordering    int        `json:"ordering"`
}

func toProgramEntryDTO(p db.ProgramEntry) programEntryDTO {
	return programEntryDTO{
		ID:          p.ID,
		EventID:     p.EventID,
		StartsAt:    p.StartsAt,
		EndsAt:      p.EndsAt,
		Title:       p.Title,
		Description: p.Description,
		Ordering:    int(p.Ordering),
	}
}

// HandleListProgram: GET /api/admin/events/{id}/program
func (s *Service) HandleListProgram(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	rows, err := s.pool.Queries().ListProgramEntries(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]programEntryDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, toProgramEntryDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

type programEntryRequest struct {
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Ordering    int        `json:"ordering"`
}

func validateProgramEntry(req programEntryRequest) error {
	if req.StartsAt.IsZero() {
		return errors.New("starts_at_required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return errors.New("title_required")
	}
	if req.EndsAt != nil && !req.EndsAt.After(req.StartsAt) {
		return errors.New("ends_at_must_be_after_starts_at")
	}
	return nil
}

// HandleCreateProgramEntry: POST /api/admin/events/{id}/program
func (s *Service) HandleCreateProgramEntry(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	var req programEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := validateProgramEntry(req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), err.Error())
		return
	}

	created, err := s.pool.Queries().CreateProgramEntry(r.Context(), db.CreateProgramEntryParams{
		EventID:     eventID,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Ordering:    int32(req.Ordering),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionEventUpdate, map[string]any{
		"program_entry_created": created.ID.String(),
	})
	writeJSON(w, http.StatusCreated, toProgramEntryDTO(created))
}

// HandleUpdateProgramEntry: PATCH /api/admin/events/{id}/program/{entryId}
func (s *Service) HandleUpdateProgramEntry(w http.ResponseWriter, r *http.Request) {
	entryID, err := uuid.Parse(chi.URLParam(r, "entryId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_entry_id", "must be a UUID")
		return
	}
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	var req programEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := validateProgramEntry(req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), err.Error())
		return
	}

	updated, err := s.pool.Queries().UpdateProgramEntry(r.Context(), db.UpdateProgramEntryParams{
		ID:          entryID,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Ordering:    int32(req.Ordering),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "program entry not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionEventUpdate, map[string]any{
		"program_entry_updated": entryID.String(),
	})
	writeJSON(w, http.StatusOK, toProgramEntryDTO(updated))
}

// HandleDeleteProgramEntry: DELETE /api/admin/events/{id}/program/{entryId}
func (s *Service) HandleDeleteProgramEntry(w http.ResponseWriter, r *http.Request) {
	entryID, err := uuid.Parse(chi.URLParam(r, "entryId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_entry_id", "must be a UUID")
		return
	}
	eventID, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}
	if err := s.pool.Queries().DeleteProgramEntry(r.Context(), entryID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.recordEventAudit(r.Context(), user.ID, eventID, audit.ActionEventUpdate, map[string]any{
		"program_entry_deleted": entryID.String(),
	})
	w.WriteHeader(http.StatusNoContent)
}
