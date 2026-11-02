package http

import (
	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/application"
)

type DetailHandler struct{ svc *a.DetailService }

func NewDetail(svc *a.DetailService) *DetailHandler { return &DetailHandler{svc} }

func (h *DetailHandler) Register(e *echo.Echo, authn echo.MiddlewareFunc) {
	e.Group("/api/v1", authn).GET("/registrations/:id", h.detail)
}

func (h *DetailHandler) detail(c echo.Context) error {
	p := c.Get("auth.profile").(auth.Profile)
	result, err := h.svc.Detail(c.Request().Context(), p.ID, c.Param("id"))
	if err != nil {
		return mapErr(c, err)
	}
	return c.JSON(200, map[string]any{"data": result})
}
