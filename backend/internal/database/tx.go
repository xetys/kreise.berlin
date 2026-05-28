package database

import (
	"context"
	"fmt"

	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// Queries returns a Queries instance bound to the pool (no transaction).
func (p *Pool) Queries() *db.Queries {
	return db.New(p.Pool)
}

// WithTx runs fn inside a database transaction. If fn returns nil the tx is
// committed; otherwise it is rolled back. The Queries handed to fn is bound
// to the transaction.
func (p *Pool) WithTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Safe to call after Commit; pgx returns ErrTxClosed which we ignore.
		_ = tx.Rollback(ctx)
	}()

	if err := fn(db.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
