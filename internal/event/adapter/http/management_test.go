package http

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
)

type managementReader struct {
	calls  int
	filter a.ManagementFilter
	kind   string
}

func (r *managementReader) ManagementList(_ context.Context, actor, event, kind string, f a.ManagementFilter) (a.ManagementPage, error) {
	r.calls++
	r.filter = f
	r.kind = kind
	return a.ManagementPage{Items: []map[string]any{}}, nil
}
func TestManagementHTTPValidation(t *testing.T) {
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"registrations", 200}, {"check-ins?method=QR", 200}, {"approval-logs", 200},
		{"registrations?page=0", 400}, {"registrations?pageSize=101", 400}, {"registrations?page=x", 400}, {"registrations?page=1&page=2", 400},
		{"registrations?status=INVALID", 400}, {"check-ins?method=invalid", 400}, {"registrations?q=x&q=y", 400}, {"registrations?page=2147483647&pageSize=100", 400},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := &managementReader{}
			e := echo.New()
			NewManagementHandler(a.NewManagementService(r)).Register(e, func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error { c.Set("auth.profile", auth.Profile{ID: "actor"}); return next(c) }
			})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/events/event/"+tc.path, nil))
			if rec.Code != tc.status {
				t.Fatal(rec.Code, rec.Body.String())
			}
			if tc.status == 400 && r.calls != 0 {
				t.Fatal("invalid query reached reader")
			}
		})
	}
	for _, kind := range []string{"registrations", "check-ins", "approval-logs"} {
		r := &managementReader{}
		e := echo.New()
		NewManagementHandler(a.NewManagementService(r)).Register(e, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error { return c.NoContent(401) }
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/events/event/"+kind, nil))
		if rec.Code != 401 || r.calls != 0 {
			t.Fatal("authentication bypass", kind)
		}
	}
}
