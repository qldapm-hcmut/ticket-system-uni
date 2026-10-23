package httpserver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/config"
	"go.uber.org/fx"
)

func TestLifecycle(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "start and stop"
		addr := "127.0.0.1:0"
		if conflict {
			name = "occupied address"
		}
		t.Run(name, func(t *testing.T) {
			if conflict {
				listener, err := net.Listen("tcp", addr)
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				addr = listener.Addr().String()
			}
			e := New(fakeDB{})
			app := fx.New(fx.NopLogger,
				fx.Supply(config.Config{HTTPAddr: addr}),
				fx.Provide(func() *echo.Echo { return e }),
				fx.Invoke(RegisterLifecycle),
			)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := app.Start(ctx)
			if conflict {
				if err == nil {
					_ = app.Stop(ctx)
					t.Fatal("expected bind error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := app.Stop(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
