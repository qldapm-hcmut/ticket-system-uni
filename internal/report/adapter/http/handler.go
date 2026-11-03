package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/report/application"
)

type Handler struct{ service *a.Service }

func New(service *a.Service) *Handler { return &Handler{service} }
func (h *Handler) Register(e *echo.Echo, authn echo.MiddlewareFunc) {
	g := e.Group("/api/v1", authn)
	g.GET("/events/:id/reports/summary", h.summary)
	g.GET("/events/:id/exports/:kind", h.export)
}
func reportError(c echo.Context, err error) error {
	status, code := 500, "INTERNAL_ERROR"
	if errors.Is(err, a.ErrNotFound) {
		status, code = 404, "NOT_FOUND"
	}
	if errors.Is(err, a.ErrValidation) {
		status, code = 400, "VALIDATION_ERROR"
	}
	return c.JSON(status, map[string]any{"error": map[string]string{"code": code, "message": http.StatusText(status)}})
}
func (h *Handler) summary(c echo.Context) error {
	p := c.Get("auth.profile").(auth.Profile)
	result, err := h.service.Summary(c.Request().Context(), p.ID, c.Param("id"))
	if err != nil {
		return reportError(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]any{"data": result})
}
func (h *Handler) export(c echo.Context) error {
	p := c.Get("auth.profile").(auth.Profile)
	data, err := h.service.Export(c.Request().Context(), p.ID, c.Param("id"), c.Param("kind"), c.QueryParam("format"))
	if err != nil {
		return reportError(c, err)
	}
	// The service has validated kind against the fixed export allowlist.
	filename := "registrations.csv"
	if c.Param("kind") == "attendance" {
		filename = "attendance.csv"
	}
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.Blob(200, "text/csv; charset=utf-8", data)
}
