// Command migrate applies database migrations using goose with the embedded migration FS.
//
// Usage:
//
//	migrate up        # apply all pending migrations
//	migrate down      # roll back the most recent migration
//	migrate status    # show applied/pending migrations
//	migrate version   # show current schema version
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/dsteiman/tickets-general/backend/internal/config"
	"github.com/dsteiman/tickets-general/backend/internal/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status|version|reset>")
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Fprintf(os.Stderr, "goose dialect: %v\n", err)
		os.Exit(1)
	}

	if err := goose.RunContext(ctx, cmd, db, ".", args...); err != nil {
		fmt.Fprintf(os.Stderr, "goose %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
