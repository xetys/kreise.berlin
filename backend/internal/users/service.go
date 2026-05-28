// Package users implements the admin lifecycle: invite + setup + role + active
// + self-service password change. Routes are mounted under /api/admin/users
// (protected) plus public /setup/{token} and /reset-password/{token}.
package users

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
)

type Config struct {
	PublicBaseURL     string
	SetupTokenTTL     time.Duration // default 7d
	ResetTokenTTL     time.Duration // default 1h
	MinPasswordChars  int           // default 12
	SessionCookieName string        // for setup-completion cookie set
	SessionTTL        time.Duration // matches auth.Config
	SecureCookie      bool
}

func DefaultConfig() Config {
	return Config{
		SetupTokenTTL:     7 * 24 * time.Hour,
		ResetTokenTTL:     1 * time.Hour,
		MinPasswordChars:  12,
		SessionCookieName: "tg_session",
		SessionTTL:        7 * 24 * time.Hour,
		SecureCookie:      true,
	}
}

type Service struct {
	pool   *database.Pool
	mailer mail.Mailer
	cfg    Config
}

func NewService(pool *database.Pool, mailer mail.Mailer, cfg Config) *Service {
	if cfg.SetupTokenTTL == 0 {
		cfg.SetupTokenTTL = 7 * 24 * time.Hour
	}
	if cfg.ResetTokenTTL == 0 {
		cfg.ResetTokenTTL = 1 * time.Hour
	}
	if cfg.MinPasswordChars == 0 {
		cfg.MinPasswordChars = 12
	}
	if cfg.SessionCookieName == "" {
		cfg.SessionCookieName = "tg_session"
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 7 * 24 * time.Hour
	}
	return &Service{pool: pool, mailer: mailer, cfg: cfg}
}

// newToken returns a URL-safe random token plus its sha256 hash (hex). The
// caller stores the hash in DB and emails the raw token to the user. When
// the user clicks the link, sha256(token) is recomputed and matched against
// the stored hash.
func newToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Service) buildSetupURL(token string) string {
	return strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/de/setup/" + token
}

func (s *Service) buildResetURL(token string) string {
	return strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/de/reset-password/" + token
}
