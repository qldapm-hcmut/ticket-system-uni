package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Pinger interface{ Ping(context.Context) error }

func New(db Pinger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.RequestID(), middleware.Recover(), middleware.BodyLimit("1M"))
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		status := http.StatusInternalServerError
		code, message := "INTERNAL_ERROR", "Internal server error."
		var he *echo.HTTPError
		if errors.As(err, &he) {
			status = he.Code
			switch status {
			case http.StatusNotFound:
				code, message = "NOT_FOUND", "Resource not found."
			case http.StatusMethodNotAllowed:
				code, message = "METHOD_NOT_ALLOWED", "Method not allowed."
			case http.StatusRequestEntityTooLarge:
				code, message = "VALIDATION_ERROR", "Request body too large."
			default:
				if status < 500 {
					code, message = "VALIDATION_ERROR", "Invalid request."
				}
			}
		}
		_ = c.JSON(status, map[string]any{"error": map[string]string{"code": code, "message": message}})
	}
	e.GET("/health/live", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
	})
	e.GET("/health/ready", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{"error": map[string]string{"code": "SERVICE_UNAVAILABLE", "message": "Service not ready."}})
		}
		return c.JSON(http.StatusOK, map[string]any{"data": map[string]string{"status": "ready"}})
	})
	return e
}
