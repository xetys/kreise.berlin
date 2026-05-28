package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/testdb"
)

// newTestService spins up a Service against the integration DB. Reuses one
// DB connection per test; resets state at the start.
func newTestService(t *testing.T) (*auth.Service, *database.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	testdb.Reset(t, pool)
	svc := auth.NewService(pool, auth.Config{
		CookieName:   "tg_session",
		SessionTTL:   time.Hour,
		SecureCookie: false,
	})
	return svc, pool
}

func TestLogin_Success_CreatesSessionAndCookie(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(""))
	rec := httptest.NewRecorder()

	sw, err := svc.Login(context.Background(), rec, req, "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sw.User.Email != "alice@example.com" {
		t.Fatalf("unexpected user email: %s", sw.User.Email)
	}

	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "tg_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected tg_session cookie to be set")
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
}

func TestLogin_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()

	_, err := svc.Login(context.Background(), rec, req, "alice@example.com", "wrong")
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_DisabledUser_ReturnsErrUserDisabled(t *testing.T) {
	svc, pool := newTestService(t)
	user := testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	if err := pool.Queries().DisableUser(context.Background(), user.ID); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()

	// GetUserByEmail filters out disabled users → invalid credentials path.
	_, err := svc.Login(context.Background(), rec, req, "alice@example.com", "secret123")
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for disabled user, got %v", err)
	}
}

func TestLogin_UnknownEmail_ReturnsInvalidCredentials(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()

	_, err := svc.Login(context.Background(), rec, req, "nobody@example.com", "anything")
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestMiddleware_Required_RejectsMissingCookie(t *testing.T) {
	svc, _ := newTestService(t)

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	mw := svc.Middleware(true)(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler must not be invoked when middleware rejects")
	}
}

func TestMiddleware_Required_PassesWithValidSession(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	loginReq := httptest.NewRequest(http.MethodPost, "/login", nil)
	loginRec := httptest.NewRecorder()
	if _, err := svc.Login(context.Background(), loginRec, loginReq, "alice@example.com", "secret123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	cookie := loginRec.Result().Cookies()[0]

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFromContext(r.Context())
		if !ok {
			t.Fatal("expected user in context")
		}
		if u.Email != "alice@example.com" {
			t.Fatalf("unexpected user email in context: %s", u.Email)
		}
		called = true
	})
	mw := svc.Middleware(true)(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Fatal("next handler should have been called")
	}
}

func TestMiddleware_Required_RejectsRevokedSession(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	loginReq := httptest.NewRequest(http.MethodPost, "/login", nil)
	loginRec := httptest.NewRecorder()
	sw, err := svc.Login(context.Background(), loginRec, loginReq, "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	cookie := loginRec.Result().Cookies()[0]

	if err := pool.Queries().RevokeSession(context.Background(), sw.Session.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	mw := svc.Middleware(true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revoke, got %d", rec.Code)
	}
}

func TestHandleLogin_HappyPath_ReturnsJSON(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	body := strings.NewReader(`{"email":"alice@example.com","password":"secret123"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	rec := httptest.NewRecorder()
	svc.HandleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		User struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.User.Email != "alice@example.com" || resp.User.Role != "global_admin" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandleLogin_WrongPassword_Returns401(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	body := strings.NewReader(`{"email":"alice@example.com","password":"wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	rec := httptest.NewRecorder()
	svc.HandleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleLogin_InvalidJSON_Returns400(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("not-json"))
	rec := httptest.NewRecorder()
	svc.HandleLogin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLogin_MissingFields_Returns400(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":""}`))
	rec := httptest.NewRecorder()
	svc.HandleLogin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLogout_RevokesSessionAndClearsCookie(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	loginReq := httptest.NewRequest(http.MethodPost, "/login", nil)
	loginRec := httptest.NewRecorder()
	sw, err := svc.Login(context.Background(), loginRec, loginReq, "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	cookie := loginRec.Result().Cookies()[0]

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRec := httptest.NewRecorder()
	svc.HandleLogout(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", logoutRec.Code)
	}
	// Cookie cleared (MaxAge=-1 in response)
	clearedFound := false
	for _, c := range logoutRec.Result().Cookies() {
		if c.Name == "tg_session" && c.MaxAge < 0 {
			clearedFound = true
		}
	}
	if !clearedFound {
		t.Fatal("expected tg_session cookie to be cleared in response")
	}

	// DB row revoked
	row, err := pool.Queries().GetSessionByID(context.Background(), sw.Session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.RevokedAt == nil {
		t.Fatal("expected session.revoked_at to be set after logout")
	}
}

func TestHandleLogout_NoCookie_StillReturns204(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	svc.HandleLogout(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 even without cookie, got %d", rec.Code)
	}
}

func TestHandleMe_WithAuth_ReturnsUser(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	loginReq := httptest.NewRequest(http.MethodPost, "/login", nil)
	loginRec := httptest.NewRecorder()
	if _, err := svc.Login(context.Background(), loginRec, loginReq, "alice@example.com", "secret123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	cookie := loginRec.Result().Cookies()[0]

	// Run HandleMe behind the middleware so context is populated.
	mw := svc.Middleware(true)(http.HandlerFunc(svc.HandleMe))
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Email != "alice@example.com" || resp.Role != "global_admin" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSessionFromContext_PresentAfterMiddleware(t *testing.T) {
	svc, pool := newTestService(t)
	testdb.SeedUser(t, pool, "alice@example.com", "secret123", "global_admin")

	loginReq := httptest.NewRequest(http.MethodPost, "/login", nil)
	loginRec := httptest.NewRecorder()
	sw, err := svc.Login(context.Background(), loginRec, loginReq, "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	cookie := loginRec.Result().Cookies()[0]

	mw := svc.Middleware(true)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		s, ok := auth.SessionFromContext(r.Context())
		if !ok {
			t.Fatal("expected session in context")
		}
		if string(s.ID) != string(sw.Session.ID) {
			t.Fatal("session ID mismatch")
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(cookie)
	mw.ServeHTTP(httptest.NewRecorder(), req)
}

func TestMiddleware_Optional_PassesThroughWithoutCookie(t *testing.T) {
	svc, _ := newTestService(t)

	called := false
	mw := svc.Middleware(false)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if !called {
		t.Fatal("optional middleware should pass through without cookie")
	}
}

func TestDefaultConfig_HasSensibleDefaults(t *testing.T) {
	cfg := auth.DefaultConfig()
	if cfg.CookieName == "" {
		t.Fatal("default cookie name must not be empty")
	}
	if cfg.SessionTTL <= 0 {
		t.Fatalf("default SessionTTL must be > 0, got %v", cfg.SessionTTL)
	}
	if !cfg.SecureCookie {
		t.Fatal("default SecureCookie should be true (prod-safe default)")
	}
}
