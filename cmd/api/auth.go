package main

import (
	"context"
	"github.com/coreos/go-oidc/v3/oidc"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	googleauth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/adapter/google"
	authhttp "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/adapter/http"
	authpg "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/adapter/postgres"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/adapter/token"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	eventhttp "github.com/qldapm-hcmut/ticket-system-uni/internal/event/adapter/http"
	eventpg "github.com/qldapm-hcmut/ticket-system-uni/internal/event/adapter/postgres"
	eventapp "github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/platform/config"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
	"go.uber.org/fx"
)

// Auth setup runs before the HTTP lifecycle hook starts accepting requests.
func registerAuth(lc fx.Lifecycle, e *echo.Echo, pool *pgxpool.Pool, appConfig config.Config) error {
	clientID, err := config.LoadGoogleClientID()
	if err != nil {
		return err
	}
	cfg, err := config.LoadAuthToken()
	if err != nil {
		return err
	}
	tokens, err := token.New(cfg.JWTSecret, kernel.SystemClock{})
	if err != nil {
		return err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		// Bound discovery/key fetches with an HTTP timeout while keeping the
		// remote JWKS context alive until shutdown.
		clientContext := oidc.ClientContext(lifetime, &http.Client{Timeout: 10 * time.Second})
		verifier, err := googleauth.New(clientContext, clientID)
		if err != nil {
			cancel()
			return err
		}
		service := application.NewService(authpg.NewUnitOfWork(pool), verifier, tokens, kernel.SystemClock{})
		authHandler := authhttp.New(service, tokens, authpg.NewProfileReader(pool))
		if appConfig.DevLoginEnabled {
			authHandler = authhttp.NewWithDevLogin(service, service, tokens, authpg.NewProfileReader(pool))
		}
		authHandler.Register(e)
		eventStore := eventpg.New(pool)
		eventReader := eventpg.NewReader(pool, kernel.SystemClock{})
		eventhttp.New(eventapp.New(eventStore, kernel.SystemClock{}), eventReader).Register(e, authHandler.Authenticate)
		eventhttp.NewApprovalHandler(eventapp.NewApprovalService(eventStore), eventReader).Register(e, authHandler.Authenticate)
		return nil
	}, OnStop: func(context.Context) error { cancel(); return nil }})
	return nil
}
