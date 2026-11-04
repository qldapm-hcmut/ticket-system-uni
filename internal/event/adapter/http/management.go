package http

import (
	"strconv"

	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

type ManagementHandler struct{ service *a.ManagementService }

func NewManagementHandler(s *a.ManagementService) *ManagementHandler { return &ManagementHandler{s} }
func (h *ManagementHandler) Register(e *echo.Echo, authenticate echo.MiddlewareFunc) {
	g := e.Group("/api/v1", authenticate)
	for _, kind := range []string{"registrations", "check-ins", "approval-logs"} {
		kind := kind
		g.GET("/events/:id/"+kind, func(c echo.Context) error { return h.list(c, kind) })
	}
}
func (h *ManagementHandler) list(c echo.Context, kind string) error {
	f := a.ManagementFilter{Page: 1, PageSize: 20}
	if kind != "approval-logs" {
		for name, dst := range map[string]*int32{"page": &f.Page, "pageSize": &f.PageSize} {
			if values, ok := c.QueryParams()[name]; ok {
				if len(values) != 1 {
					return failure(c, domain.ErrValidation)
				}
				n, err := strconv.ParseInt(values[0], 10, 32)
				if err != nil {
					return failure(c, domain.ErrValidation)
				}
				*dst = int32(n)
			}
		}
		names := []string{"status", "q"}
		if kind == "check-ins" {
			names = []string{"method"}
		}
		for _, name := range names {
			values := c.QueryParams()[name]
			if len(values) > 1 {
				return failure(c, domain.ErrValidation)
			}
		}
		if kind == "registrations" {
			f.Status = c.QueryParam("status")
			f.Search = c.QueryParam("q")
		} else {
			f.Method = c.QueryParam("method")
		}
	}
	out, err := h.service.List(c.Request().Context(), c.Get("auth.profile").(auth.Profile).ID, c.Param("id"), kind, f)
	if err != nil {
		return failure(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	body := map[string]any{"data": out.Items}
	if kind != "approval-logs" {
		body["meta"] = map[string]any{"page": f.Page, "pageSize": f.PageSize, "totalItems": out.Total, "totalPages": (out.Total + int64(f.PageSize) - 1) / int64(f.PageSize)}
	}
	return c.JSON(200, body)
}
