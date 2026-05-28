// Package domain holds the application-layer types decoupled from the sqlc row
// types in internal/db. Handlers and services should accept and return these
// rather than db.* directly.
package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
)

type Role string

const (
	RoleGlobalAdmin  Role = "global_admin"
	RoleEventAdmin   Role = "event_admin"
	RoleEventManager Role = "event_manager"
)

func (r Role) Valid() bool {
	switch r {
	case RoleGlobalAdmin, RoleEventAdmin, RoleEventManager:
		return true
	}
	return false
}

type User struct {
	ID              uuid.UUID
	Email           string
	Role            Role
	DisplayName     string
	CreatedAt       time.Time
	DisabledAt      *time.Time
	Active          bool
	PasswordVersion int32
	LastLoginAt     *time.Time
}

func (u User) IsActive() bool {
	return u.DisabledAt == nil && u.Active
}

func UserFromDB(u db.User) User {
	displayName := ""
	if u.DisplayName != nil {
		displayName = *u.DisplayName
	}
	return User{
		ID:              u.ID,
		Email:           u.Email,
		Role:            Role(u.Role),
		DisplayName:     displayName,
		CreatedAt:       u.CreatedAt,
		DisabledAt:      u.DisabledAt,
		Active:          u.Active,
		PasswordVersion: u.PasswordVersion,
		LastLoginAt:     u.LastLoginAt,
	}
}
