package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
)

type fakeService struct{ logoutID string }

func (*fakeService) Login(context.Context, string) (application.LoginResult, error) {
	return application.LoginResult{User: application.User{ID: "user"}, Tokens: application.Tokens{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 900}}, nil
}
func (*fakeService) Refresh(context.Context, string) (application.Tokens, error) {
	return application.Tokens{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 900}, nil
}
func (s *fakeService) Logout(_ context.Context, id, _ string) error { s.logoutID = id; return nil }

type fakeAccess struct{}

func (fakeAccess) VerifyAccessToken(raw string) (string, error) {
	if raw != "valid" {
		return "", application.ErrUnauthenticated
	}
	return "user", nil
}

type fakeProfiles struct{ disabled bool }

func (p *fakeProfiles) ReadProfile(context.Context, string) (application.Profile, error) {
	if p.disabled {
		return application.Profile{}, application.ErrUnauthenticated
	}
	return application.Profile{User: application.User{ID: "user", Email: "test@hcmut.edu.vn", IsActive: true, StudentCode: "2210001"}, Memberships: []application.Membership{{UnitID: "unit", MemberRole: "ORGANIZER"}}}, nil
}

func TestHTTPRoutes(t *testing.T) {
	e := echo.New()
	s := &fakeService{}
	p := &fakeProfiles{}
	New(s, fakeAccess{}, p).Register(e)
	for _, tc := range []struct {
		method, path, body, auth string
		status                   int
	}{
		{"POST", "/api/v1/auth/google", `{"idToken":"google"}`, "", 200},
		{"POST", "/api/v1/auth/refresh", `{"refreshToken":"refresh"}`, "", 200},
		{"POST", "/api/v1/auth/google", `{}`, "", 400},
		{"POST", "/api/v1/auth/refresh", `{"refreshToken":42}`, "", 400},
		{"POST", "/api/v1/auth/google", `{"idToken":"x"} {}`, "", 400},
		{"GET", "/api/v1/me", "", "", 401},
		{"GET", "/api/v1/me", "", "Bearer invalid", 401},
		{"GET", "/api/v1/me", "", "Bearer valid", 200},
		{"POST", "/api/v1/auth/logout", `{"refreshToken":"refresh"}`, "Bearer valid", 204},
	} {
		t.Run(tc.method+tc.path+tc.auth+tc.body, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", tc.auth)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if rec.Code == 200 {
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload["data"] == nil {
					t.Fatal("missing data envelope")
				}
			}
		})
	}
	if s.logoutID != "user" {
		t.Fatal("logout not scoped to authenticated user")
	}
	p.disabled = true
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("disabled user accepted")
	}
}
