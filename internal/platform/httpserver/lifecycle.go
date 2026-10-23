package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/config"
	"go.uber.org/fx"
)

// RegisterLifecycle registers HTTP startup and graceful shutdown hooks.
func RegisterLifecycle(lc fx.Lifecycle, cfg config.Config, e *echo.Echo, shutdown fx.Shutdowner) {
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 15 * time.Second
	e.Server.WriteTimeout = 30 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			// Bind synchronously so address conflicts fail application startup.
			listener, err := net.Listen("tcp", cfg.HTTPAddr)
			if err != nil {
				return fmt.Errorf("listen HTTP: %w", err)
			}
			e.Listener = listener
			go func() {
				defer close(done)
				if err := e.Start(cfg.HTTPAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
					slog.Error("HTTP server failed", "error", err)
					_ = shutdown.Shutdown(fx.ExitCode(1))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := e.Shutdown(ctx); err != nil {
				_ = e.Close()
				return fmt.Errorf("shutdown HTTP: %w", err)
			}
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}
