// Command bootstrap-admin idempotently creates the first global_admin on a
// fresh deployment. Runs as a Helm post-install/post-upgrade Job:
//
//   - reads ADMIN_EMAIL + ADMIN_PASSWORD + ADMIN_DISPLAY_NAME from the env
//   - if any global_admin already exists in the DB, exits 0 silently
//   - otherwise hashes the password with auth.HashPassword (argon2id) and
//     inserts a global_admin row with active=true
//
// Idempotent: re-runs are no-ops once a global_admin exists. Designed to
// replace the manual SQL seed that hand-grants the first privileged account
// on a brand-new cluster.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dsn := os.Getenv("DATABASE_URL")
	email := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	displayName := strings.TrimSpace(os.Getenv("ADMIN_DISPLAY_NAME"))

	if dsn == "" || email == "" || password == "" {
		logger.Error("DATABASE_URL, ADMIN_EMAIL and ADMIN_PASSWORD are required")
		os.Exit(2)
	}
	if !strings.Contains(email, "@") {
		logger.Error("ADMIN_EMAIL must contain @")
		os.Exit(2)
	}
	if len(password) < 12 {
		logger.Error("ADMIN_PASSWORD must be at least 12 characters")
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		logger.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := pool.Queries()

	// Idempotency: bail if a global_admin already exists.
	n, err := q.CountGlobalAdmins(ctx)
	if err != nil {
		logger.Error("count global_admins failed", "err", err)
		os.Exit(1)
	}
	if n > 0 {
		logger.Info("a global_admin already exists, nothing to do", "count", n)
		return
	}

	// Re-invite case: if a row with this email already exists (from a prior
	// failed attempt or manual seed) overwrite role + password rather than
	// erroring. Keeps re-runs safe.
	hash, err := auth.HashPassword(password)
	if err != nil {
		logger.Error("hash password failed", "err", err)
		os.Exit(1)
	}

	existing, err := q.GetUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		logger.Error("lookup existing user failed", "err", err)
		os.Exit(1)
	}
	if err == nil {
		// Promote existing row to global_admin and set the password.
		if err := q.SetUserRole(ctx, db.SetUserRoleParams{ID: existing.ID, Role: "global_admin"}); err != nil {
			logger.Error("promote existing user failed", "err", err)
			os.Exit(1)
		}
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: existing.ID, PasswordHash: hash}); err != nil {
			logger.Error("set password on existing user failed", "err", err)
			os.Exit(1)
		}
		if err := q.ReactivateUser(ctx, existing.ID); err != nil {
			logger.Error("reactivate existing user failed", "err", err)
			os.Exit(1)
		}
		logger.Info("promoted existing user to global_admin", "user_id", existing.ID, "email", email)
		return
	}

	var displayNamePtr *string
	if displayName != "" {
		displayNamePtr = &displayName
	}
	created, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		Role:         "global_admin",
		DisplayName:  displayNamePtr,
	})
	if err != nil {
		logger.Error("create user failed", "err", err)
		os.Exit(1)
	}
	logger.Info("first global_admin created", "user_id", created.ID, "email", created.Email)
	fmt.Println("OK")
}
