// Package worker manages the background worker lifecycle.
package worker

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

// RegisterLifecycle registers worker lifecycle hooks.
// Requiring the pool ensures its hooks precede worker hooks. Jobs will stop
// before the pool closes (Fx stops hooks in reverse order).
func RegisterLifecycle(lc fx.Lifecycle, _ *pgxpool.Pool) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			slog.Info("worker skeleton started; no jobs registered")
			return nil
		},
	})
}
