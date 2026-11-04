package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestJobRunsImmediatelyRetriesAndStops(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{}, 4)
	app := fxtest.New(t, fx.Invoke(func(lc fx.Lifecycle) {
		register(lc, []job{{"test", 5 * time.Millisecond, func(ctx context.Context) (int64, error) {
			calls.Add(1)
			select {
			case entered <- struct{}{}:
			default:
			}
			return 0, errors.New("transient")
		}}})
	}))
	app.RequireStart()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("job did not start/retry")
		}
	}
	app.RequireStop()
	before := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != before {
		t.Fatal("job ran after shutdown")
	}
}
func TestShutdownCancelsActiveJob(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	app := fxtest.New(t, fx.Invoke(func(lc fx.Lifecycle) {
		register(lc, []job{{"blocking", time.Hour, func(ctx context.Context) (int64, error) {
			close(entered)
			<-ctx.Done()
			close(exited)
			return 0, ctx.Err()
		}}})
	}))
	app.RequireStart()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("job not started")
	}
	app.RequireStop()
	select {
	case <-exited:
	default:
		t.Fatal("job not stopped")
	}
}
