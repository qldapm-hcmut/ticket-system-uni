// Command worker declares the background worker dependency graph.
package main

import (
	"time"

	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/config"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/postgres"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/worker"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.Provide(config.Load, postgres.NewPool),
		fx.Invoke(worker.RegisterLifecycle),
		fx.StartTimeout(15*time.Second),
		fx.StopTimeout(10*time.Second),
	).Run()
}
