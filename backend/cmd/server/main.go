package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/authz"
	"github.com/dsteiman/tickets-general/backend/internal/booking"
	"github.com/dsteiman/tickets-general/backend/internal/config"
	"github.com/dsteiman/tickets-general/backend/internal/csrf"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/events"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
	"github.com/dsteiman/tickets-general/backend/internal/objectstore"
	"github.com/dsteiman/tickets-general/backend/internal/tickets"
	"github.com/dsteiman/tickets-general/backend/internal/users"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.Env)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("db connected")

	// Apply pending migrations on startup. Goose tracks state in
	// goose_db_version, so this is idempotent. Multi-replica safe via
	// goose's per-step advisory lock.
	if err := database.Migrate(ctx, pool.Pool); err != nil {
		logger.Error("db migrate failed", "err", err)
		os.Exit(1)
	}
	logger.Info("db migrations applied")

	authSvc := auth.NewService(pool, auth.Config{
		CookieName:   cfg.SessionCookieName,
		SessionTTL:   cfg.SessionTTL,
		SecureCookie: cfg.IsProd(),
	})

	mailer, err := mail.New(ctx, pool, mail.Config{
		From:                cfg.MailFrom,
		FromDisplayName:     cfg.MailFromDisplayName,
		UnsubscribeEmail:    cfg.MailUnsubscribeAddress,
		Region:              cfg.AWSRegion,
		ConfigurationSetARN: cfg.SESConfigurationSet,
	})
	if err != nil {
		logger.Error("mailer init failed", "err", err)
		os.Exit(1)
	}

	objStore, err := objectstore.New(ctx, objectstore.Config{
		Endpoint:     cfg.StorageEndpoint,
		Region:       cfg.StorageRegion,
		Bucket:       cfg.StorageBucket,
		AccessKey:    cfg.StorageAccessKey,
		SecretKey:    cfg.StorageSecretKey,
		UsePathStyle: cfg.StorageUsePathStyle,
	})
	if err != nil {
		logger.Error("objectstore init failed", "err", err)
		os.Exit(1)
	}

	guard := authz.NewHTTPGuard(pool)
	eventsSvc := events.NewService(pool, objStore)
	bookingSvc := booking.NewService(pool, mailer, booking.Config{
		ReservationTTL:  7 * 24 * time.Hour,
		PublicBaseURL:   cfg.PublicBaseURL,
		TokenSigningKey: cfg.TokenSigningKey,
	})

	go bookingSvc.RunSweeper(ctx, logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(logging.Middleware(logger))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		pingCtx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			logging.FromContext(req.Context()).Warn("readyz: db ping failed", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("db unreachable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.RedirectSlashes)
		r.Use(csrf.Middleware(cfg.IsProd()))

		r.Get("/csrf", csrf.HandleBootstrap)

		// Public read endpoints — no auth required, but live under /api so
		// they share the CSRF middleware (which only enforces on unsafe methods).
		events.MountPublicAPI(r, eventsSvc)

		// Public booking flow (POST /quote, POST /bookings).
		booking.MountPublic(r, bookingSvc)

		// Public ticket-page surface (GET /tickets/{token}, cancel, transfer, qr.png).
		ticketsSvc := tickets.NewService(pool, mailer, tickets.Config{
			TokenSigningKey: cfg.TokenSigningKey,
			PublicBaseURL:   cfg.PublicBaseURL,
		})
		// Per-ticket cancels free a seat — fire waitlist promotion.
		ticketsSvc.SetOnCancelHook(func(hookCtx context.Context, eventID uuid.UUID, hookLogger *slog.Logger) {
			if err := bookingSvc.PromoteAfterCancel(hookCtx, eventID, hookLogger); err != nil {
				hookLogger.Warn("waitlist promotion after ticket cancel failed", "err", err, "event_id", eventID)
			}
		})
		tickets.MountPublic(r, ticketsSvc)

		usersSvc := users.NewService(pool, mailer, users.Config{
			PublicBaseURL:     cfg.PublicBaseURL,
			SessionCookieName: cfg.SessionCookieName,
			SessionTTL:        cfg.SessionTTL,
			SecureCookie:      cfg.IsProd(),
		})
		users.MountPublic(r, usersSvc)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", authSvc.HandleLogin)

			r.Group(func(r chi.Router) {
				r.Use(authSvc.Middleware(true))
				r.Post("/logout", authSvc.HandleLogout)
				r.Get("/me", authSvc.HandleMe)
				r.Post("/stop-impersonating", authSvc.HandleStopImpersonating)
			})
		})

		// Authenticated admin endpoints (events, bookings, users, mail test, ...).
		r.Group(func(r chi.Router) {
			r.Use(authSvc.Middleware(true))

			events.MountAdmin(r, eventsSvc, guard)
			booking.MountAdmin(r, bookingSvc)
			users.MountAdmin(r, usersSvc)
			r.Post("/admin/users/{id}/impersonate", authSvc.HandleImpersonate)

			r.Post("/admin/mail/test", makeMailTestHandler(mailer, logger))
		})
	})

	// Public banner pass-through (no auth, no CSRF).
	events.MountPublic(r, eventsSvc)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", cfg.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("server shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "err", err)
		os.Exit(1)
	}
}

// makeMailTestHandler sends a bilingual test email through the configured
// mailer. Used to verify SMTP→mailcatcher in dev and SES in higher envs.
func makeMailTestHandler(mailer mail.Mailer, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			To string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.To == "" {
			http.Error(w, "to is required", http.StatusBadRequest)
			return
		}

		user, _ := auth.UserFromContext(r.Context())
		msg, err := mail.RenderBilingual("mail.test", mail.BilingualSpec{
			SubjectDE: "Testmail",
			BodyDE:    "Hallo {{.Name}},\n\ndies ist ein Test der Mailer-Konfiguration.",
			SubjectEN: "Test email",
			BodyEN:    "Hi {{.Name}},\n\nthis is a test of the mailer configuration.",
		}, mail.TemplateData{"Name": user.Email})
		if err != nil {
			logger.Error("render test mail failed", "err", err)
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		msg.To = req.To

		if err := mailer.Send(r.Context(), msg); err != nil {
			logger.Error("test mail send failed", "err", err)
			http.Error(w, "send failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
