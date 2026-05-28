package authz

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

// These tests cover the role × action matrix without hitting the DB. Cases
// that depend on team membership are deferred to the integration test
// package (which uses a real Postgres).
func TestAllow_RoleActionMatrix(t *testing.T) {
	platform := Platform()
	user := func(role domain.Role) domain.User {
		return domain.User{ID: uuid.New(), Role: role, Email: "t@x"}
	}

	cases := []struct {
		name    string
		user    domain.User
		action  Action
		res     Resource
		wantErr bool
	}{
		// Global admin allowed everywhere
		{"global_admin/event_create/platform", user(domain.RoleGlobalAdmin), ActionEventCreate, platform, false},
		{"global_admin/booking_refund/platform", user(domain.RoleGlobalAdmin), ActionBookingRefund, platform, false},

		// Event admin: platform actions
		{"event_admin/event_create/platform", user(domain.RoleEventAdmin), ActionEventCreate, platform, false},
		{"event_admin/booking_refund/platform", user(domain.RoleEventAdmin), ActionBookingRefund, platform, false},

		// Event manager: cannot create events
		{"event_manager/event_create/platform", user(domain.RoleEventManager), ActionEventCreate, platform, true},
		// Event manager: cannot refund (action denied at role level, never reaches membership check)
		{"event_manager/booking_refund/platform", user(domain.RoleEventManager), ActionBookingRefund, platform, true},
		{"event_manager/newsletter/platform", user(domain.RoleEventManager), ActionNewsletterSend, platform, true},

		// Event manager: actions allowed at role level (but membership not checked here since platform scope)
		{"event_manager/booking_list/platform", user(domain.RoleEventManager), ActionBookingList, platform, false},
		{"event_manager/booking_check_in/platform", user(domain.RoleEventManager), ActionBookingCheckIn, platform, false},
		{"event_manager/booking_mark_paid/platform", user(domain.RoleEventManager), ActionBookingMarkPaid, platform, false},

		// Disabled global_admin → denied
		func() struct {
			name    string
			user    domain.User
			action  Action
			res     Resource
			wantErr bool
		} {
			now := time.Now()
			u := user(domain.RoleGlobalAdmin)
			u.DisabledAt = &now
			return struct {
				name    string
				user    domain.User
				action  Action
				res     Resource
				wantErr bool
			}{"disabled_global_admin/any/platform", u, ActionEventCreate, platform, true}
		}(),
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Allow(context.Background(), nil, tc.user, tc.action, tc.res)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
}

func TestRolePermissions_NoAccidentalGlobalAdminEntry(t *testing.T) {
	// Global admin is handled by an early return in Allow, not via the
	// rolePermissions map. A row for it would be confusing and dangerous.
	if _, ok := rolePermissions[domain.RoleGlobalAdmin]; ok {
		t.Fatal("rolePermissions must not contain RoleGlobalAdmin (Allow short-circuits)")
	}
}
