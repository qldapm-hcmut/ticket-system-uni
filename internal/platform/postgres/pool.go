package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/config"
	"go.uber.org/fx"
)

// NewPool creates a pool and registers its lifecycle hooks.
// Consumers must not close the pool.
func NewPool(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	// Pool construction is lazy; network checks belong to OnStart.
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool failed")
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				// Fx does not call OnStop for a hook whose OnStart failed.
				pool.Close()
				return fmt.Errorf("postgres: database unavailable")
			}
			return nil
		},
		OnStop: func(context.Context) error { pool.Close(); return nil },
	})
	return pool, nil
}
