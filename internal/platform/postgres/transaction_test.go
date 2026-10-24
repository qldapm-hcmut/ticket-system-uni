package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeTx struct {
	pgx.Tx
	commitErr, rollbackErr error
	commits, rollbacks     int
	rollbackContextErr     error
}

func (tx *fakeTx) Commit(context.Context) error {
	tx.commits++
	return tx.commitErr
}

func (tx *fakeTx) Rollback(ctx context.Context) error {
	tx.rollbacks++
	tx.rollbackContextErr = ctx.Err()
	return tx.rollbackErr
}

func runnerFor(tx *fakeTx) *TransactionRunner {
	return &TransactionRunner{begin: func(context.Context) (pgx.Tx, error) { return tx, nil }}
}

func TestTransactionRunnerDo(t *testing.T) {
	failure := errors.New("failure")
	cleanupFailure := errors.New("cleanup failure")
	for _, tt := range []struct {
		name                                string
		callbackErr, commitErr, rollbackErr error
		wantErr                             error
		wantCommits                         int
	}{
		{"success", nil, nil, pgx.ErrTxClosed, nil, 1},
		{"callback failure", failure, nil, nil, failure, 0},
		{"commit failure", nil, failure, nil, failure, 1},
		{"closed rollback", failure, nil, pgx.ErrTxClosed, failure, 0},
		{"rollback failure", failure, nil, cleanupFailure, failure, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeTx{commitErr: tt.commitErr, rollbackErr: tt.rollbackErr}
			ctx := context.WithValue(context.Background(), struct{}{}, "value")
			err := runnerFor(tx).Do(ctx, func(gotCtx context.Context, gotTx pgx.Tx) error {
				if gotCtx != ctx || gotTx != tx {
					t.Fatal("callback received different context or transaction")
				}
				return tt.callbackErr
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.rollbackErr == cleanupFailure && !errors.Is(err, cleanupFailure) {
				t.Fatalf("lost rollback error: %v", err)
			}
			if tx.commits != tt.wantCommits || tx.rollbacks != 1 {
				t.Fatalf("commits=%d rollbacks=%d", tx.commits, tx.rollbacks)
			}
		})
	}
}

func TestTransactionRunnerBeginFailure(t *testing.T) {
	failure := errors.New("begin failed")
	r := &TransactionRunner{begin: func(context.Context) (pgx.Tx, error) { return nil, failure }}
	err := r.Do(context.Background(), func(context.Context, pgx.Tx) error { t.Fatal("callback must not run"); return nil })
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
}

func TestTransactionRunnerCancelledRequestStillRollsBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tx := &fakeTx{}
	err := runnerFor(tx).Do(ctx, func(context.Context, pgx.Tx) error { cancel(); return ctx.Err() })
	if !errors.Is(err, context.Canceled) || tx.rollbackContextErr != nil || tx.rollbacks != 1 || tx.commits != 0 {
		t.Fatalf("error=%v rollback context=%v commits=%d rollbacks=%d", err, tx.rollbackContextErr, tx.commits, tx.rollbacks)
	}
}

func TestTransactionRunnerPanicRollsBack(t *testing.T) {
	tx := &fakeTx{}
	defer func() {
		if got := recover(); got != "boom" {
			t.Errorf("panic = %v", got)
		}
		if tx.rollbacks != 1 || tx.commits != 0 {
			t.Errorf("commits=%d rollbacks=%d", tx.commits, tx.rollbacks)
		}
	}()
	_ = runnerFor(tx).Do(context.Background(), func(context.Context, pgx.Tx) error { panic("boom") })
}
