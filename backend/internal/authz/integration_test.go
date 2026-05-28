package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dsteiman/tickets-general/backend/internal/authz"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/testdb"
)

// These tests cover the parts of Allow that touch the DB (event_admin /
// event_manager membership lookups). The pure role × action matrix is
// covered by authz_test.go without a DB.
func TestAllow_EventAdmin_RequiresMembership(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)

	owner := testdb.SeedUser(t, pool, "owner@example.com", "", "global_admin")
	assigned := testdb.SeedUser(t, pool, "assigned@example.com", "", "event_admin")
	stranger := testdb.SeedUser(t, pool, "stranger@example.com", "", "event_admin")

	event := testdb.SeedEvent(t, pool, "test-event", owner.ID)
	testdb.AssignEventAdmin(t, pool, event.ID, assigned.ID)

	q := pool.Queries()
	res := authz.ForEvent(event.ID)

	// Assigned event_admin is allowed
	if err := authz.Allow(context.Background(), q, domain.UserFromDB(assigned), authz.ActionEventUpdate, res); err != nil {
		t.Fatalf("expected assigned event_admin to be allowed, got %v", err)
	}

	// Stranger event_admin is forbidden
	err := authz.Allow(context.Background(), q, domain.UserFromDB(stranger), authz.ActionEventUpdate, res)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for unassigned event_admin, got %v", err)
	}
}

func TestAllow_EventManager_RequiresManagerAssignment(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)

	owner := testdb.SeedUser(t, pool, "owner@example.com", "", "global_admin")
	manager := testdb.SeedUser(t, pool, "manager@example.com", "", "event_manager")
	stranger := testdb.SeedUser(t, pool, "stranger@example.com", "", "event_manager")

	event := testdb.SeedEvent(t, pool, "test-event", owner.ID)
	testdb.AssignEventManager(t, pool, event.ID, manager.ID)

	q := pool.Queries()
	res := authz.ForEvent(event.ID)

	if err := authz.Allow(context.Background(), q, domain.UserFromDB(manager), authz.ActionBookingMarkPaid, res); err != nil {
		t.Fatalf("assigned manager should be allowed: %v", err)
	}
	if err := authz.Allow(context.Background(), q, domain.UserFromDB(stranger), authz.ActionBookingMarkPaid, res); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("stranger manager should be forbidden, got %v", err)
	}
}

func TestAllow_EventManager_DeniedForRoleScopedActions(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)

	owner := testdb.SeedUser(t, pool, "owner@example.com", "", "global_admin")
	manager := testdb.SeedUser(t, pool, "manager@example.com", "", "event_manager")
	event := testdb.SeedEvent(t, pool, "test-event", owner.ID)
	testdb.AssignEventManager(t, pool, event.ID, manager.ID)

	q := pool.Queries()
	res := authz.ForEvent(event.ID)

	// Even with team membership, event_manager cannot perform event_admin actions.
	cases := []authz.Action{
		authz.ActionEventUpdate,
		authz.ActionEventArchive,
		authz.ActionPricingEdit,
		authz.ActionBookingRefund,
		authz.ActionNewsletterSend,
	}
	for _, a := range cases {
		a := a
		t.Run(string(a), func(t *testing.T) {
			err := authz.Allow(context.Background(), q, domain.UserFromDB(manager), a, res)
			if !errors.Is(err, authz.ErrForbidden) {
				t.Fatalf("expected ErrForbidden for manager doing %s, got %v", a, err)
			}
		})
	}
}

func TestAllow_GlobalAdmin_BypassesMembership(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)

	root := testdb.SeedUser(t, pool, "root@example.com", "", "global_admin")
	other := testdb.SeedUser(t, pool, "other@example.com", "", "global_admin")
	event := testdb.SeedEvent(t, pool, "test-event", other.ID) // root is NOT assigned

	q := pool.Queries()
	res := authz.ForEvent(event.ID)

	if err := authz.Allow(context.Background(), q, domain.UserFromDB(root), authz.ActionEventUpdate, res); err != nil {
		t.Fatalf("global_admin should bypass membership: %v", err)
	}
}
