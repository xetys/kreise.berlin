// Package authz exposes a single Allow(user, action, resource) entry point that
// every handler must route through. Inline role checks elsewhere are a smell:
// the action table here is the only authorization policy in the system.
package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

var ErrForbidden = errors.New("forbidden")

// Action names a verb the caller wants to perform. Adding a new action means
// adding a row in rolePermissions and (if event-scoped) ensuring callers pass
// ForEvent(id) when they invoke Allow.
type Action string

const (
	// Event lifecycle (event_admin scope, except Create which is platform-level).
	ActionEventCreate     Action = "event.create"
	ActionEventRead       Action = "event.read"
	ActionEventUpdate     Action = "event.update"
	ActionEventArchive    Action = "event.archive"
	ActionEventPublish    Action = "event.publish"
	ActionEventAddAdmin   Action = "event.add_admin"
	ActionEventAddManager Action = "event.add_manager"

	// Pricing & coupons (event_admin scope).
	ActionPricingEdit Action = "pricing.edit"

	// Booking management (split: event_manager can mark_paid + check_in;
	// only event_admin can refund).
	ActionBookingList     Action = "booking.list"
	ActionBookingMarkPaid Action = "booking.mark_paid"
	ActionBookingRefund   Action = "booking.refund"
	ActionBookingCheckIn  Action = "booking.check_in"

	// Newsletter (event_admin scope).
	ActionNewsletterSend Action = "newsletter.send"
)

// Resource describes the target of the action. EventID is set for event-scoped
// actions; nil means platform-level (and applies only to actions that make
// sense without an event, like ActionEventCreate).
type Resource struct {
	EventID *uuid.UUID
}

func Platform() Resource { return Resource{} }

func ForEvent(id uuid.UUID) Resource {
	return Resource{EventID: &id}
}

// rolePermissions defines which actions each role can _ever_ perform. Event
// scope is enforced separately via team-membership lookups in Allow.
var rolePermissions = map[domain.Role]map[Action]bool{
	domain.RoleEventAdmin: {
		ActionEventCreate:     true, // platform-level
		ActionEventRead:       true,
		ActionEventUpdate:     true,
		ActionEventArchive:    true,
		ActionEventPublish:    true,
		ActionEventAddAdmin:   true,
		ActionEventAddManager: true,
		ActionPricingEdit:     true,
		ActionBookingList:     true,
		ActionBookingMarkPaid: true,
		ActionBookingRefund:   true,
		ActionBookingCheckIn:  true,
		ActionNewsletterSend:  true,
	},
	domain.RoleEventManager: {
		ActionEventRead:       true,
		ActionBookingList:     true,
		ActionBookingMarkPaid: true,
		// Managers cancel/refund bookings as part of normal door + before-event
		// operations (no-show fallout, last-minute cancellations). They never
		// touch event metadata (description, dates, banner) — that stays
		// admin-only via ActionEventUpdate.
		ActionBookingRefund:  true,
		ActionBookingCheckIn: true,
	},
}

// Allow returns nil when user may perform action on res. ErrForbidden when
// disallowed by role or by team membership. Other errors come from the DB.
//
// Global admins bypass all checks. Event-scoped actions for event_admin /
// event_manager require the user to be assigned to that event's team.
func Allow(ctx context.Context, q *db.Queries, user domain.User, action Action, res Resource) error {
	if !user.IsActive() {
		return ErrForbidden
	}
	if user.Role == domain.RoleGlobalAdmin {
		return nil
	}
	perms, ok := rolePermissions[user.Role]
	if !ok || !perms[action] {
		return ErrForbidden
	}
	if res.EventID == nil {
		return nil
	}

	var member bool
	var err error
	switch user.Role {
	case domain.RoleEventAdmin:
		member, err = q.IsEventAdmin(ctx, db.IsEventAdminParams{
			EventID: *res.EventID,
			UserID:  user.ID,
		})
	case domain.RoleEventManager:
		member, err = q.IsEventManager(ctx, db.IsEventManagerParams{
			EventID: *res.EventID,
			UserID:  user.ID,
		})
	default:
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("membership check: %w", err)
	}
	if !member {
		return ErrForbidden
	}
	return nil
}
