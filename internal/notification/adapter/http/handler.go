package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/notification/application"
)

type Handler struct{ service *a.Service }

func New(s *a.Service) *Handler { return &Handler{s} }
func (h *Handler) Register(e *echo.Echo, authenticate echo.MiddlewareFunc) {
	g := e.Group("/api/v1/me/notifications", authenticate)
	g.GET("", h.list)
	g.POST("/read", h.read)
}
func failure(c echo.Context, err error) error {
	status, code := 500, "INTERNAL_ERROR"
	if errors.Is(err, a.ErrValidation) {
		status, code = 400, "VALIDATION_ERROR"
	}
	return c.JSON(status, map[string]any{"error": map[string]string{"code": code, "message": http.StatusText(status)}})
}
func (h *Handler) list(c echo.Context) error {
	f := a.Filter{Page: 1, PageSize: 20}
	for name, dst := range map[string]*int32{"page": &f.Page, "pageSize": &f.PageSize} {
		if raw, ok := c.QueryParams()[name]; ok {
			if len(raw) != 1 {
				return failure(c, a.ErrValidation)
			}
			n, err := strconv.ParseInt(raw[0], 10, 32)
			if err != nil {
				return failure(c, a.ErrValidation)
			}
			*dst = int32(n)
		}
	}
	if raw, ok := c.QueryParams()["unread"]; ok {
		if len(raw) != 1 || raw[0] != "true" && raw[0] != "false" {
			return failure(c, a.ErrValidation)
		}
		f.Unread = raw[0] == "true"
	}
	result, err := h.service.List(c.Request().Context(), c.Get("auth.profile").(auth.Profile).ID, f)
	if err != nil {
		return failure(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(200, map[string]any{"data": result.Items, "meta": map[string]any{"page": f.Page, "pageSize": f.PageSize, "totalItems": result.TotalItems, "totalPages": (result.TotalItems + int64(f.PageSize) - 1) / int64(f.PageSize), "unreadCount": result.UnreadCount}})
}
func (h *Handler) read(c echo.Context) error {
	kind, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		return failure(c, a.ErrValidation)
	}
	var body struct {
		IDs *[]string `json:"ids"`
		All *bool     `json:"all"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 16384))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return failure(c, a.ErrValidation)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return failure(c, a.ErrValidation)
	}
	if (body.IDs == nil) == (body.All == nil) || body.All != nil && !*body.All {
		return failure(c, a.ErrValidation)
	}
	options := a.ReadOptions{All: body.All != nil}
	if body.IDs != nil {
		options.IDs = *body.IDs
	}
	result, err := h.service.MarkRead(c.Request().Context(), c.Get("auth.profile").(auth.Profile).ID, options)
	if err != nil {
		return failure(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(200, map[string]any{"data": result})
}
