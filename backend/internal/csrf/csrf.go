// Package csrf implements double-submit cookie CSRF protection.
//
// Safe methods (GET/HEAD/OPTIONS) issue or refresh a cookie containing a
// random token. State-changing methods require the request's X-CSRF-Token
// header to match the cookie value (constant-time compare).
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

const (
	CookieName = "tg_csrf"
	HeaderName = "X-CSRF-Token"
)

// Middleware returns the CSRF guard. secureCookie controls the Secure attribute
// on the issued cookie (true in prod; false in dev where TLS is absent).
func Middleware(secureCookie bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			haveCookie := err == nil && cookie.Value != ""

			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				if !haveCookie {
					token, err := newToken()
					if err != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					http.SetCookie(w, &http.Cookie{
						Name:     CookieName,
						Value:    token,
						Path:     "/",
						SameSite: http.SameSiteLaxMode,
						Secure:   secureCookie,
						// Intentionally NOT HttpOnly: the frontend reads this
						// cookie and copies the value into the X-CSRF-Token
						// header on state-changing requests.
					})
				}
			default:
				if !haveCookie {
					http.Error(w, "csrf cookie missing", http.StatusForbidden)
					return
				}
				supplied := r.Header.Get(HeaderName)
				if supplied == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(cookie.Value)) != 1 {
					http.Error(w, "csrf check failed", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HandleBootstrap is mounted at GET /api/csrf so the frontend can request a
// fresh CSRF cookie before showing a form. The middleware sets the cookie
// automatically as a side effect of a GET; this handler just confirms.
func HandleBootstrap(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
