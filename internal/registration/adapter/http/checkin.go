package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/application"
)

type CheckinHandler struct{ store a.CheckinStore }

func NewCheckin(store a.CheckinStore) *CheckinHandler { return &CheckinHandler{store} }
func (h *CheckinHandler) Register(e *echo.Echo, authn echo.MiddlewareFunc) {
	g := e.Group("/api/v1", authn)
	g.POST("/events/:id/check-ins", h.checkin)
	g.DELETE("/registrations/:id/check-in", h.undo)
}
func checkinError(c echo.Context, err error) error {
	var duplicate *a.AlreadyCheckedIn
	if errors.As(err, &duplicate) {
		return c.JSON(409, map[string]any{"error": map[string]any{"code": "ALREADY_CHECKED_IN", "message": "Already checked in", "details": map[string]any{"checkedInAt": duplicate.At, "checkedInBy": duplicate.By}}})
	}
	status, code := 0, ""
	switch {
	case errors.Is(err, a.ErrSelfCheckin):
		status, code = 403, "SELF_CHECKIN_NOT_ALLOWED"
	case errors.Is(err, a.ErrCheckinWindow):
		status, code = 422, "CHECKIN_WINDOW_CLOSED"
	case errors.Is(err, a.ErrCheckinEligibility):
		status, code = 422, "NOT_ELIGIBLE_FOR_CHECKIN"
	}
	if status != 0 {
		return c.JSON(status, map[string]any{"error": map[string]string{"code": code, "message": http.StatusText(status)}})
	}
	return mapErr(c, err)
}
func (h *CheckinHandler) checkin(c echo.Context) error {
	p := c.Get("auth.profile").(auth.Profile)
	if len(p.Memberships) == 0 {
		return mapErr(c, a.ErrForbidden)
	}
	media, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return mapErr(c, a.ErrValidation)
	}
	var in a.CheckinInput
	dec := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 16384))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return mapErr(c, a.ErrValidation)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return mapErr(c, a.ErrValidation)
	}
	result, err := h.store.CheckIn(c.Request().Context(), p.ID, c.Param("id"), in)
	if err != nil {
		return checkinError(c, err)
	}
	return c.JSON(201, map[string]any{"data": result})
}
func (h *CheckinHandler) undo(c echo.Context) error {
	p := c.Get("auth.profile").(auth.Profile)
	if len(p.Memberships) == 0 {
		return mapErr(c, a.ErrForbidden)
	}
	if err := h.store.UndoCheckIn(c.Request().Context(), p.ID, c.Param("id")); err != nil {
		return checkinError(c, err)
	}
	return c.NoContent(204)
}
