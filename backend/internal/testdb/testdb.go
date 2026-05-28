// Package testdb provides shared helpers for integration tests that need a
// real Postgres connection. Tests skip themselves if TEST_DATABASE_URL is
// unset, so unit tests still run anywhere.
//
// Tests that share this DB must NOT run in parallel — Reset truncates all
// tables, so concurrent tests would tread on each other.
package testdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// dataTables enumerates user-data tables truncated between tests. Schema
// changes that add tables must be reflected here or older tests will
// inherit state.
var dataTables = []string{
	"audit_log",
	"email_log",
	"waitlist_entries",
	"tickets",
	"participants",
	"coupon_redemptions",
	"bookings",
	"coupon_category_filters",
	"coupon_phase_filters",
	"coupons",
	"donation_configs",
	"prices",
	"price_durations",
	"price_categories",
	"price_phases",
	"program_entries",
	"event_managers",
	"event_admins",
	"events",
	"sessions",
	"users",
}

// Pool opens a connection pool to TEST_DATABASE_URL and returns it. The pool
// is closed automatically when the test ends. If the env var is unset the
// test is skipped — keeps unit tests viable in environments without Postgres.
func Pool(t *testing.T) *database.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	p, err := database.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test DB: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

// Reset truncates every data table. Call at the start of each test that
// needs a clean slate.
func Reset(t *testing.T, pool *database.Pool) {
	t.Helper()
	sql := "TRUNCATE " + strings.Join(quoted(dataTables), ", ") + " RESTART IDENTITY CASCADE"
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("reset test DB: %v", err)
	}
}

// SeedUser inserts a user with a freshly hashed password and returns it.
// password defaults to "password" when empty.
func SeedUser(t *testing.T, pool *database.Pool, email, password, role string) db.User {
	t.Helper()
	if password == "" {
		password = "password"
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	created, err := pool.Queries().CreateUser(context.Background(), db.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		Role:         role,
		DisplayName:  nilIfEmpty("Test " + role),
	})
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return created
}

// SeedEvent inserts a minimal event owned by createdBy.
func SeedEvent(t *testing.T, pool *database.Pool, slug string, createdBy uuid.UUID) db.Event {
	t.Helper()
	q := pool.Queries()
	// Direct INSERT via Exec; we don't have a CreateEvent query yet (Phase 2).
	row, err := q.GetEventBySlug(context.Background(), slug)
	if err == nil {
		return row
	}
	const sql = `
		INSERT INTO events (slug, name, starts_at, ends_at, pricing_mode, created_by, is_public)
		VALUES ($1, $1, now() + interval '1 day', now() + interval '2 days', 'matrix', $2, false)
		RETURNING id
	`
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), sql, slug, createdBy).Scan(&id); err != nil {
		t.Fatalf("seed event %s: %v", slug, err)
	}
	row, err = q.GetEventByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get seeded event: %v", err)
	}
	return row
}

// AssignEventAdmin links userID as an admin on eventID.
func AssignEventAdmin(t *testing.T, pool *database.Pool, eventID, userID uuid.UUID) {
	t.Helper()
	const sql = `INSERT INTO event_admins (event_id, user_id) VALUES ($1, $2)`
	if _, err := pool.Exec(context.Background(), sql, eventID, userID); err != nil {
		t.Fatalf("assign event admin: %v", err)
	}
}

// AssignEventManager links userID as a manager on eventID.
func AssignEventManager(t *testing.T, pool *database.Pool, eventID, userID uuid.UUID) {
	t.Helper()
	const sql = `INSERT INTO event_managers (event_id, user_id) VALUES ($1, $2)`
	if _, err := pool.Exec(context.Background(), sql, eventID, userID); err != nil {
		t.Fatalf("assign event manager: %v", err)
	}
}

func quoted(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = `"` + n + `"`
	}
	return out
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
