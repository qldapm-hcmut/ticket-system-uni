// Package worker manages cancellable background jobs through Fx lifecycle.
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.uber.org/fx"
)

type Jobs interface {
	ExpireWaitlists(context.Context) (int64, error)
	CompleteEvents(context.Context) (int64, error)
}

type job struct {
	name     string
	interval time.Duration
	run      func(context.Context) (int64, error)
}

// Jobs run immediately on startup, then on their own schedule. A slow run
// never overlaps another run of the same job. Database locks coordinate replicas.
func RegisterLifecycle(lc fx.Lifecycle, jobs Jobs) {
	register(lc, []job{
		{"expire_waitlists", 5 * time.Minute, jobs.ExpireWaitlists},
		{"complete_events", 15 * time.Minute, jobs.CompleteEvents},
	})
}
func register(lc fx.Lifecycle, jobs []job) {
	var cancel context.CancelFunc
	var done chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			done = make(chan struct{})
			var wg sync.WaitGroup
			for _, j := range jobs {
				wg.Add(1)
				go func(j job) { defer wg.Done(); runJob(ctx, j) }(j)
			}
			go func() { wg.Wait(); close(done) }()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cancel == nil {
				return nil
			}
			cancel()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}
func runJob(ctx context.Context, j job) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		// Bound blocked queries and lock waits; retry failed work at the next tick.
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		changed, err := j.run(runCtx)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("worker job failed", "job", j.name, "error", err)
			}
		} else if changed > 0 {
			slog.Info("worker job completed", "job", j.name, "changed", changed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
