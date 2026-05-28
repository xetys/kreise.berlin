// Package audit centralizes writes to the audit_log table. Every admin
// mutation should call Record exactly once with a stable action key.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// Stable action keys. Keep these grouped by domain object; Phase 2+ will add
// rows here as new admin operations land.
const (
	ActionUserCreate   = "user.create"
	ActionUserDisable  = "user.disable"
	ActionUserPassword = "user.password"

	ActionEventCreate     = "event.create"
	ActionEventUpdate     = "event.update"
	ActionEventArchive    = "event.archive"
	ActionEventPublish    = "event.publish"
	ActionEventUnpublish  = "event.unpublish"
	ActionEventAddAdmin   = "event.add_admin"
	ActionEventAddManager = "event.add_manager"

	ActionBookingMarkPaid = "booking.mark_paid"
	ActionBookingRefund   = "booking.refund"
	ActionBookingCheckIn  = "booking.check_in"

	ActionPricingUpdate = "pricing.update"
	ActionCouponCreate  = "coupon.create"

	ActionNewsletterSend = "newsletter.send"
)

// Common target_kind values.
const (
	TargetUser    = "user"
	TargetEvent   = "event"
	TargetBooking = "booking"
	TargetTicket  = "ticket"
	TargetCoupon  = "coupon"
)

// Entry is the input shape for Record.
type Entry struct {
	ActorUserID *uuid.UUID
	EventID     *uuid.UUID
	Action      string
	TargetKind  string
	TargetID    string // empty when not applicable (e.g. global config edit)
	Payload     any    // any JSON-serializable value; may be nil
}

// Record writes one audit_log row. Marshaling errors abort the write.
//
// Caller convention: errors from Record should not block the parent operation
// — log them and proceed. Audit failures are observability incidents, not
// business failures.
func Record(ctx context.Context, pool *database.Pool, e Entry) error {
	var payload []byte
	if e.Payload != nil {
		b, err := json.Marshal(e.Payload)
		if err != nil {
			return fmt.Errorf("marshal audit payload: %w", err)
		}
		payload = b
	}
	var targetID *string
	if e.TargetID != "" {
		t := e.TargetID
		targetID = &t
	}
	return pool.Queries().InsertAuditLog(ctx, db.InsertAuditLogParams{
		ActorUserID: e.ActorUserID,
		EventID:     e.EventID,
		Action:      e.Action,
		TargetKind:  e.TargetKind,
		TargetID:    targetID,
		Payload:     payload,
	})
}
