package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionRunner owns the transaction lifecycle for PostgreSQL adapters.
// Keep pgx types in adapters: application UnitOfWork implementations should
// bind their repositories to the supplied tx before invoking application code.
// Each Do starts an independent transaction; nested calls are not savepoints.
// Callbacks must not commit, roll back, retain tx, or perform external I/O.
// There is no automatic retry: a failed commit can have an ambiguous outcome.
type TransactionRunner struct {
	begin func(context.Context) (pgx.Tx, error)
}

func NewTransactionRunner(pool *pgxpool.Pool) *TransactionRunner {
	return &TransactionRunner{begin: pool.Begin}
}

// Do commits only when fn succeeds. Errors retain their identity for errors.Is
// and errors.As. Panics propagate after a best-effort rollback.
func (r *TransactionRunner) Do(ctx context.Context, fn func(context.Context, pgx.Tx) error) (err error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// Request cancellation must not prevent cleanup. Bound cleanup so a
		// broken connection cannot hold a pool slot indefinitely.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		rollbackErr := tx.Rollback(cleanupCtx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
