package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/report/application"
)

type fakeStore struct {
	err                error
	calls              int
	actor, event, kind string
}

func (f *fakeStore) Summary(_ context.Context, actor, event string) (a.Summary, error) {
	f.calls++
	f.actor, f.event = actor, event
	return a.Summary{Capacity: 100, ByMethod: map[string]int64{"QR": 0, "CODE": 0, "MANUAL": 0}}, f.err
}
func (f *fakeStore) Export(_ context.Context, actor, event, kind string) ([]byte, error) {
	f.calls++
	f.actor, f.event, f.kind = actor, event, kind
	return []byte("csv data"), f.err
}
func setup(store *fakeStore, authenticated bool) *echo.Echo {
	e := echo.New()
	New(a.New(store)).Register(e, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !authenticated {
				return c.NoContent(http.StatusUnauthorized)
			}
			c.Set("auth.profile", auth.Profile{ID: "actor"})
			return next(c)
		}
	})
	return e
}
func TestReportHTTP(t *testing.T) {
	for _, tc := range []struct {
		path   string
		err    error
		status int
	}{
		{"reports/summary", nil, 200}, {"exports/registrations", nil, 200}, {"exports/attendance?format=csv", nil, 200},
		{"exports/invalid", nil, 400}, {"exports/attendance?format=xlsx", nil, 400},
		{"reports/summary", a.ErrNotFound, 404}, {"exports/attendance", a.ErrNotFound, 404},
		{"reports/summary", errors.New("sensitive database detail"), 500}, {"exports/attendance", errors.New("sensitive database detail"), 500},
	} {
		t.Run(tc.path+http.StatusText(tc.status), func(t *testing.T) {
			f := &fakeStore{err: tc.err}
			e := setup(f, true)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/events/event/"+tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if tc.status == 400 {
				if f.calls != 0 {
					t.Fatal("invalid export reached store")
				}
			} else if f.actor != "actor" || f.event != "event" {
				t.Fatalf("scope lost: %+v", f)
			}
			if tc.status >= 400 {
				var body struct {
					Error struct{ Code, Message string }
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				want := map[int]string{400: "VALIDATION_ERROR", 404: "NOT_FOUND", 500: "INTERNAL_ERROR"}[tc.status]
				if body.Error.Code != want || body.Error.Message == "sensitive database detail" {
					t.Fatal(rec.Body.String())
				}
				if rec.Header().Get("Content-Disposition") != "" {
					t.Fatal("error returned attachment header")
				}
			} else {
				if rec.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("report must not be cached")
				}
				if tc.path != "reports/summary" {
					filename := "registrations.csv"
					if f.kind == "attendance" {
						filename = "attendance.csv"
					}
					if rec.Header().Get("Content-Disposition") != `attachment; filename="`+filename+`"` || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Body.String() != "csv data" {
						t.Fatalf("export: %v %s", rec.Header(), rec.Body.String())
					}
				}
			}
		})
	}
}
func TestReportRoutesRequireAuthentication(t *testing.T) {
	f := &fakeStore{}
	e := setup(f, false)
	for _, path := range []string{"reports/summary", "exports/attendance", "exports/registrations"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/events/event/"+path, nil))
		if rec.Code != 401 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	if f.calls != 0 {
		t.Fatal("unauthenticated request reached store")
	}
}
