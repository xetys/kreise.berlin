package csrf

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestGetIssuesCookie(t *testing.T) {
	mw := Middleware(false)(nopHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	setCookie := rec.Result().Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, CookieName+"=") {
		t.Fatalf("expected %s cookie in response, got %q", CookieName, setCookie)
	}
}

func TestPostWithoutCookieIsForbidden(t *testing.T) {
	mw := Middleware(false)(nopHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestPostWithMatchingHeaderPasses(t *testing.T) {
	mw := Middleware(false)(nopHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "abc123"})
	req.Header.Set(HeaderName, "abc123")

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPostWithMismatchedHeaderIsForbidden(t *testing.T) {
	mw := Middleware(false)(nopHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "abc123"})
	req.Header.Set(HeaderName, "different")

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestGetWithExistingCookieDoesNotReissue(t *testing.T) {
	mw := Middleware(false)(nopHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "preexisting"})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if c := rec.Result().Header.Get("Set-Cookie"); c != "" {
		t.Fatalf("did not expect Set-Cookie when cookie already present, got %q", c)
	}
}
