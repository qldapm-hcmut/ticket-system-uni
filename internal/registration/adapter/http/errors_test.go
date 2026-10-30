package http

import (
	"errors"
	"github.com/labstack/echo/v4"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/application"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{a.ErrValidation, 400, "VALIDATION_ERROR"}, {a.ErrState, 409, "INVALID_STATE_TRANSITION"}, {a.ErrCancelDeadline, 422, "CANCEL_DEADLINE_PASSED"}, {a.ErrNotFound, 404, "NOT_FOUND"}, {a.ErrAlreadyActive, 409, "ALREADY_REGISTERED"}, {a.ErrFull, 409, "EVENT_FULL"}, {a.ErrClosed, 422, "REGISTRATION_CLOSED"}, {errors.New("private database detail"), 500, "INTERNAL_ERROR"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest("GET", "/", nil), rec)
			if err := mapErr(c, tc.err); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.code) || strings.Contains(rec.Body.String(), "private database detail") {
				t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
