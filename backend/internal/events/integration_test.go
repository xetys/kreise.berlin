package events_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/authz"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/events"
	"github.com/dsteiman/tickets-general/backend/internal/testdb"
)

// newRouter builds a fully-wired router around events.MountAdmin so tests
// exercise the same middleware stack as production (authz guard included).
func newRouter(t *testing.T, pool *database.Pool) (chi.Router, *events.Service) {
	t.Helper()
	svc := events.NewService(pool, nil) // no objectstore in tests
	guard := authz.NewHTTPGuard(pool)

	r := chi.NewRouter()
	events.MountAdmin(r, svc, guard)
	return r, svc
}

// runAs adds the user to the request context as if auth middleware ran.
func runAs(t *testing.T, r chi.Router, user db.User, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	var req *http.Request
	if bodyReader != nil {
		req = httptest.NewRequest(method, path, bodyReader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")

	ctx := auth.WithSessionUser(req.Context(), domain.SessionWithUser{
		User: domain.UserFromDB(user),
	})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

const validCreate = `{
  "slug": "%s",
  "name": "Test",
  "description": "",
  "starts_at": "2026-06-01T19:00:00Z",
  "ends_at": "2026-06-01T22:00:00Z",
  "pricing_mode": "matrix"
}`

func createBody(slug string) string {
	return strings.Replace(validCreate, "%s", slug, 1)
}

// ----------------------------------------------------------------------------

func TestCreateEvent_AsEventAdmin_AutoAssignsCreator(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	creator := testdb.SeedUser(t, pool, "creator@x", "", "event_admin")

	rec := runAs(t, router, creator, http.MethodPost, "/admin/events", createBody("dao-dance"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Creator should now be in event_admins.
	id := extractEventID(t, rec.Body.String())
	is, err := pool.Queries().IsEventAdmin(context.Background(), db.IsEventAdminParams{
		EventID: id,
		UserID:  creator.ID,
	})
	if err != nil {
		t.Fatalf("check assignment: %v", err)
	}
	if !is {
		t.Fatal("expected creator to be auto-assigned as event_admin")
	}
}

func TestCreateEvent_AsEventManager_Forbidden(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	mgr := testdb.SeedUser(t, pool, "mgr@x", "", "event_manager")

	rec := runAs(t, router, mgr, http.MethodPost, "/admin/events", createBody("nope"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListEvents_ScopedByRole(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	root := testdb.SeedUser(t, pool, "root@x", "", "global_admin")
	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")
	bob := testdb.SeedUser(t, pool, "bob@x", "", "event_admin")

	// Alice creates an event.
	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("alices-event"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create event: %d %s", rec.Code, rec.Body.String())
	}

	// Bob lists — should see nothing.
	rec = runAs(t, router, bob, http.MethodGet, "/admin/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Fatalf("expected empty event list for bob, got %s", rec.Body.String())
	}

	// Root sees Alice's event.
	rec = runAs(t, router, root, http.MethodGet, "/admin/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "alices-event") {
		t.Fatalf("expected global_admin to see alice's event, got %s", rec.Body.String())
	}

	// Alice sees her own.
	rec = runAs(t, router, alice, http.MethodGet, "/admin/events", "")
	if !strings.Contains(rec.Body.String(), "alices-event") {
		t.Fatalf("expected alice to see her own event, got %s", rec.Body.String())
	}
}

func TestUpdateEvent_OtherEventAdmin_Forbidden(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")
	bob := testdb.SeedUser(t, pool, "bob@x", "", "event_admin")

	// Alice creates her event.
	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("alice-evt"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id := extractField(rec.Body.String(), "id")

	updateBody := `{
      "name": "Hacked",
      "description": "",
      "color_primary": "#000000",
      "color_secondary": "#ffffff",
      "color_text": "#000000",
      "location": "",
      "starts_at": "2026-06-01T19:00:00Z",
      "ends_at": "2026-06-01T22:00:00Z",
      "pricing_mode": "matrix",
      "currency": "EUR",
      "default_locale": "de"
    }`

	// Bob tries to update Alice's event — should be 403.
	rec = runAs(t, router, bob, http.MethodPatch, "/admin/events/"+strings.Trim(id, `"`), updateBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign event update, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPublishEvent_RequiresPermission(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")
	bob := testdb.SeedUser(t, pool, "bob@x", "", "event_admin")
	manager := testdb.SeedUser(t, pool, "mgr@x", "", "event_manager")

	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("publish-test"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id := strings.Trim(extractField(rec.Body.String(), "id"), `"`)

	// Foreign event_admin → 403.
	rec = runAs(t, router, bob, http.MethodPost, "/admin/events/"+id+"/publish", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign publish, got %d", rec.Code)
	}
	// Manager → 403 (they don't have ActionEventPublish).
	rec = runAs(t, router, manager, http.MethodPost, "/admin/events/"+id+"/publish", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for manager publish, got %d", rec.Code)
	}
	// Owner → 200.
	rec = runAs(t, router, alice, http.MethodPost, "/admin/events/"+id+"/publish", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for owner publish, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestArchiveEvent_RequiresPermission(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")

	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("archive-test"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id := strings.Trim(extractField(rec.Body.String(), "id"), `"`)

	rec = runAs(t, router, alice, http.MethodPost, "/admin/events/"+id+"/archive", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_archived":true`) {
		t.Fatalf("expected is_archived=true, got %s", rec.Body.String())
	}
}

func TestCreateEvent_InvalidSlug_Returns400(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")
	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("Bad Slug With Spaces"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid slug, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateEvent_DuplicateSlug_Returns409(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")

	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("dupe"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d %s", rec.Code, rec.Body.String())
	}
	rec = runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("dupe"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditLog_RecordedOnCreate(t *testing.T) {
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	router, _ := newRouter(t, pool)

	alice := testdb.SeedUser(t, pool, "alice@x", "", "event_admin")
	rec := runAs(t, router, alice, http.MethodPost, "/admin/events", createBody("audit-test"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'event.create'`).Scan(&count); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event.create audit row, got %d", count)
	}
}

// ----------------------------------------------------------------------------

// extractField returns the json-encoded value for the named top-level field.
// Used to read scalar fields out of handler responses in tests.
func extractField(jsonBody, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonBody), &m); err != nil {
		return ""
	}
	b, _ := json.Marshal(m[field])
	return string(b)
}

// extractEventID parses the event id out of a JSON body, fatal on error.
func extractEventID(t *testing.T, jsonBody string) uuid.UUID {
	t.Helper()
	var m struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal([]byte(jsonBody), &m); err != nil {
		t.Fatalf("decode id: %v", err)
	}
	return m.ID
}
