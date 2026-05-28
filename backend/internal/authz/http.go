package authz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// HTTPGuard wraps Allow with HTTP-level concerns: extracting the user from
// the request context, optionally resolving a target event from a URL
// parameter, and translating Allow's outcomes into the canonical API error
// shape.
type HTTPGuard struct {
	pool *database.Pool
}

func NewHTTPGuard(pool *database.Pool) *HTTPGuard {
	return &HTTPGuard{pool: pool}
}

// Require gates a route on action with no event scope.
func (g *HTTPGuard) Require(action Action) func(http.Handler) http.Handler {
	return g.guard(action, nil)
}

// RequireForEventParam reads the event UUID from the chi URL parameter
// `paramName` and gates the route on action × event membership.
//
// Hands a 400 to the client when the param is missing or not a UUID.
func (g *HTTPGuard) RequireForEventParam(action Action, paramName string) func(http.Handler) http.Handler {
	return g.guard(action, func(r *http.Request) (uuid.UUID, error) {
		raw := chi.URLParam(r, paramName)
		if raw == "" {
			return uuid.Nil, errEventParamMissing
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, errEventParamInvalid
		}
		return id, nil
	})
}

func (g *HTTPGuard) guard(
	action Action,
	resolveEvent func(*http.Request) (uuid.UUID, error),
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.UserFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "no session")
				return
			}

			res := Platform()
			if resolveEvent != nil {
				id, err := resolveEvent(r)
				if err != nil {
					switch {
					case errors.Is(err, errEventParamMissing):
						writeError(w, http.StatusBadRequest, "missing_event_id", "event id missing in URL")
					case errors.Is(err, errEventParamInvalid):
						writeError(w, http.StatusBadRequest, "invalid_event_id", "event id is not a valid UUID")
					default:
						writeError(w, http.StatusBadRequest, "invalid_event_id", err.Error())
					}
					return
				}
				res = ForEvent(id)
			}

			err := Allow(r.Context(), g.pool.Queries(), user, action, res)
			switch {
			case err == nil:
				next.ServeHTTP(w, r)
			case errors.Is(err, ErrForbidden):
				writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to "+string(action))
			default:
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			}
		})
	}
}

// Q returns a Queries handle bound to the guard's pool. Useful for handlers
// that already have to import authz for the middleware and don't want a
// separate database dependency.
func (g *HTTPGuard) Q() *db.Queries {
	return g.pool.Queries()
}

var (
	errEventParamMissing = errors.New("event id param missing")
	errEventParamInvalid = errors.New("event id param invalid")
)

func writeError(w http.ResponseWriter, status int, code, devMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":             code,
		"developer_message": devMessage,
	})
}

// EventIDFromContext is a small placeholder for a future feature where a
// nested handler wants to know the event id resolved by the guard. Not
// wired today; reserved.
func EventIDFromContext(_ context.Context) (uuid.UUID, bool) {
	return uuid.Nil, false
}
