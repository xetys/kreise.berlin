package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
)

type Session struct {
	ID         []byte
	UserID     uuid.UUID
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

func (s Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}

func SessionFromDB(s db.Session) Session {
	return Session{
		ID:         s.ID,
		UserID:     s.UserID,
		CreatedAt:  s.CreatedAt,
		LastSeenAt: s.LastSeenAt,
		ExpiresAt:  s.ExpiresAt,
		RevokedAt:  s.RevokedAt,
	}
}

// SessionWithUser is what the join query produces on session lookup.
type SessionWithUser struct {
	Session Session
	User    User

	// ImpersonatorUserID + ImpersonatorEmail are populated when the session
	// was minted by a global_admin via the impersonation endpoint. Pure UI /
	// audit signal — authz still runs against User (the effective user).
	ImpersonatorUserID *uuid.UUID
	ImpersonatorEmail  string
}

// SessionWithUserFromRow constructs a SessionWithUser from the sqlc row of
// GetSessionByID (which selects session + joined user fields).
func SessionWithUserFromRow(r db.GetSessionByIDRow) SessionWithUser {
	displayName := ""
	if r.UserDisplayName != nil {
		displayName = *r.UserDisplayName
	}
	impEmail := ""
	if r.ImpersonatorEmail != nil {
		impEmail = *r.ImpersonatorEmail
	}
	return SessionWithUser{
		Session: Session{
			ID:         r.ID,
			UserID:     r.UserID,
			CreatedAt:  r.CreatedAt,
			LastSeenAt: r.LastSeenAt,
			ExpiresAt:  r.ExpiresAt,
			RevokedAt:  r.RevokedAt,
		},
		User: User{
			ID:              r.UserID,
			Email:           r.UserEmail,
			Role:            Role(r.UserRole),
			DisplayName:     displayName,
			DisabledAt:      r.UserDisabledAt,
			Active:          r.UserActive,
			PasswordVersion: r.UserPasswordVersion,
			// CreatedAt is not selected by the join — leave zero. Not consumed by callers.
		},
		ImpersonatorUserID: r.ImpersonatorUserID,
		ImpersonatorEmail:  impEmail,
	}
}
