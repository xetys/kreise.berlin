// Package booking implements the public booking flow: live quote previews
// (POST /api/quote) and the actual booking transaction (POST /api/bookings)
// that creates a booking + participants + tickets in one shot, redeems any
// coupon under a row lock, and fires the bilingual confirmation email.
//
// Reservation expiry on unpaid bookings is handled by an in-process sweeper
// (RunSweeper) that ticks every minute.
package booking

import (
	"time"

	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
	"github.com/dsteiman/tickets-general/backend/internal/pricing"
)

// Config bundles the knobs the booking flow needs.
type Config struct {
	// ReservationTTL is how long a `booked` (unpaid) booking holds its
	// seats before the sweeper cancels it. Defaults to 7 days — bank
	// transfers need real time. Donation events ignore this entirely.
	ReservationTTL time.Duration

	// PublicBaseURL is used to build per-ticket view links in the
	// confirmation email (e.g. {{base}}/de/tickets/{viewToken}).
	PublicBaseURL string

	// TokenSigningKey is the HMAC key for QR + view tokens. Required.
	TokenSigningKey []byte
}

func DefaultConfig() Config {
	return Config{
		ReservationTTL: 7 * 24 * time.Hour,
	}
}

type Service struct {
	pool   *database.Pool
	mailer mail.Mailer
	repo   pricing.Repo
	cfg    Config
}

func NewService(pool *database.Pool, mailer mail.Mailer, cfg Config) *Service {
	if cfg.ReservationTTL == 0 {
		cfg.ReservationTTL = 7 * 24 * time.Hour
	}
	return &Service{
		pool:   pool,
		mailer: mailer,
		repo:   pricing.NewStore(pool.Queries()),
		cfg:    cfg,
	}
}
