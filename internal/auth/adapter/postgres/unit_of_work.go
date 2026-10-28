package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	platformpg "github.com/qldapm-hcmut/ticket-system-uni/internal/platform/postgres"
)

// UnitOfWork adapts PostgreSQL transactions to the authentication application port.
type UnitOfWork struct {
	runner  *platformpg.TransactionRunner
	queries *db.Queries
}

var _ application.UnitOfWork = (*UnitOfWork)(nil)

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{runner: platformpg.NewTransactionRunner(pool), queries: db.New(pool)}
}

func (u *UnitOfWork) Do(ctx context.Context, fn func(context.Context, application.Repository) error) error {
	// The runner owns the single transaction; this adapter binds the repository.
	return u.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		repo := &repository{queries: u.queries.WithTx(tx)}
		return fn(ctx, repo)
	})
}
