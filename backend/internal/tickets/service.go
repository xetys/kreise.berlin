// Package tickets implements the public per-ticket self-service surface:
// magic-link ticket page, cancel, transfer, and the QR PNG for door scanning.
//
// All endpoints sit behind a signed view-purpose token (see internal/tokens).
// No auth session involved — possession of the token is the credential.
package tickets

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
)

type Config struct {
	TokenSigningKey []byte
	PublicBaseURL   string
}

// OnCancelHook is invoked best-effort after a ticket cancel or transfer
// frees up capacity. The booking package wires this to its waitlist
// promote-after-cancel logic so per-ticket cancels rotate the waitlist.
//
// Called in a goroutine — no return value, errors are this hook's problem.
type OnCancelHook func(ctx context.Context, eventID uuid.UUID, logger *slog.Logger)

type Service struct {
	pool     *database.Pool
	mailer   mail.Mailer
	cfg      Config
	onCancel OnCancelHook
}

func NewService(pool *database.Pool, mailer mail.Mailer, cfg Config) *Service {
	return &Service{pool: pool, mailer: mailer, cfg: cfg}
}

// SetOnCancelHook wires the post-cancel callback (typically the booking
// package's PromoteAfterCancel). Idempotent — passing nil disables the hook.
func (s *Service) SetOnCancelHook(h OnCancelHook) {
	s.onCancel = h
}
