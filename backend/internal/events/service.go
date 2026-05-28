// Package events implements the admin-side CRUD for events: create, list
// (scoped by role), update, archive, publish/unpublish, and the team
// assignment endpoints. Banner upload and program-entry handlers live in
// sibling files.
package events

import (
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/objectstore"
)

// Service bundles the dependencies the events handlers need.
type Service struct {
	pool        *database.Pool
	objectstore *objectstore.Client // optional; nil = banner upload disabled
}

func NewService(pool *database.Pool, store *objectstore.Client) *Service {
	return &Service{pool: pool, objectstore: store}
}
