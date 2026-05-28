package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

// Config bundles the knobs the auth service needs.
type Config struct {
	CookieName   string        // e.g. "tg_session"
	SessionTTL   time.Duration // e.g. 7 * 24h
	SecureCookie bool          // false in dev (no TLS), true otherwise
}

func DefaultConfig() Config {
	return Config{
		CookieName:   "tg_session",
		SessionTTL:   7 * 24 * time.Hour,
		SecureCookie: true,
	}
}

type Service struct {
	pool *database.Pool
	cfg  Config
}

func NewService(pool *database.Pool, cfg Config) *Service {
	if cfg.CookieName == "" {
		cfg.CookieName = "tg_session"
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 7 * 24 * time.Hour
	}
	return &Service{pool: pool, cfg: cfg}
}

// Login verifies credentials, opens a session, and writes the session cookie.
// Returns the resulting session+user pair on success.
func (s *Service) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, email, password string) (domain.SessionWithUser, error) {
	q := s.pool.Queries()

	row, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SessionWithUser{}, ErrInvalidCredentials
		}
		return domain.SessionWithUser{}, fmt.Errorf("get user: %w", err)
	}
	if row.DisabledAt != nil {
		return domain.SessionWithUser{}, ErrUserDisabled
	}
	if !row.Active {
		// Invited but not yet completed setup — block login. Setup-token flow
		// is the only legitimate path to first authentication.
		return domain.SessionWithUser{}, ErrUserDisabled
	}
	ok, vErr := VerifyPassword(password, row.PasswordHash)
	if vErr != nil {
		return domain.SessionWithUser{}, ErrInvalidCredentials
	}
	if !ok {
		return domain.SessionWithUser{}, ErrInvalidCredentials
	}

	sessionID, err := newSessionID()
	if err != nil {
		return domain.SessionWithUser{}, fmt.Errorf("session id: %w", err)
	}

	ua := truncate(r.UserAgent(), 500)
	ip := remoteAddrIP(r)

	created, err := q.CreateSession(ctx, db.CreateSessionParams{
		ID:              sessionID,
		UserID:          row.ID,
		ExpiresAt:       time.Now().Add(s.cfg.SessionTTL),
		UserAgent:       nilIfEmpty(ua),
		Ip:              ip,
		PasswordVersion: row.PasswordVersion,
	})
	if err != nil {
		return domain.SessionWithUser{}, fmt.Errorf("create session: %w", err)
	}

	s.setCookie(w, sessionID, created.ExpiresAt)

	return domain.SessionWithUser{
		Session: domain.SessionFromDB(created),
		User:    domain.UserFromDB(row),
	}, nil
}

// Logout revokes the session referenced by the cookie (if any) and clears the cookie.
func (s *Service) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	id, err := s.readSessionID(r)
	if err == nil {
		if err := s.pool.Queries().RevokeSession(ctx, id); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
	}
	s.clearCookie(w)
	return nil
}

// Middleware validates the session cookie and attaches the user to the request
// context. When required is true, requests without a valid session get a 401.
func (s *Service) Middleware(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := s.readSessionID(r)
			if err != nil {
				if required {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			row, err := s.pool.Queries().GetSessionByID(r.Context(), id)
			if err != nil {
				if required {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			now := time.Now()
			// Reject the session if any of these is true:
			//   - explicitly revoked
			//   - past expiry
			//   - user disabled
			//   - user not active (invited-but-never-completed-setup)
			//   - session.password_version != user.password_version
			//     (force-logout / password change happened after this session
			//     was minted; the bumped version invalidates every prior session
			//     in one move).
			stale := row.SessionPasswordVersion != row.UserPasswordVersion
			if row.RevokedAt != nil || !row.ExpiresAt.After(now) || row.UserDisabledAt != nil || !row.UserActive || stale {
				s.clearCookie(w)
				if required {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Best-effort touch; don't fail the request if the touch errors.
			_ = s.pool.Queries().TouchSession(r.Context(), id)

			sw := domain.SessionWithUserFromRow(row)
			ctx := WithSessionUser(r.Context(), sw)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ----- internal helpers -----

func newSessionID() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func encodeSessionID(id []byte) string {
	return base64.RawURLEncoding.EncodeToString(id)
}

func decodeSessionID(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func (s *Service) setCookie(w http.ResponseWriter, id []byte, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    encodeSessionID(id),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookie,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
}

func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Service) readSessionID(r *http.Request) ([]byte, error) {
	c, err := r.Cookie(s.cfg.CookieName)
	if err != nil {
		return nil, ErrNoSession
	}
	id, err := decodeSessionID(c.Value)
	if err != nil || len(id) != 32 {
		return nil, ErrNoSession
	}
	return id, nil
}

func remoteAddrIP(r *http.Request) *netip.Addr {
	host := r.RemoteAddr
	// strip optional :port
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			host = host[:i]
			break
		}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &addr
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
