// Command api declares the dependency graph; Fx owns startup and shutdown.
package main

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/config"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/httpserver"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/postgres"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			postgres.NewPool,
			func(pool *pgxpool.Pool) *echo.Echo { return httpserver.New(pool) },
		),
		fx.Invoke(registerAuth, httpserver.RegisterLifecycle),
		fx.StartTimeout(15*time.Second),
		fx.StopTimeout(10*time.Second),
	).Run()
}
