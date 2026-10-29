//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	authhttp "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/adapter/http"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/adapter/token"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
)

func testHTTPFlow(t *testing.T, pool *pgxpool.Pool, uow *UnitOfWork) {
	tokens, err := token.New("test-only-key-at-least-32-bytes-long", kernel.SystemClock{})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(uow, identityStub{application.Identity{Sub: "http-sub", Email: "http@hcmut.edu.vn", FullName: "HTTP"}}, tokens, kernel.SystemClock{})
	e := echo.New()
	authhttp.New(service, tokens, NewProfileReader(pool)).Register(e)
	request := func(method, path, body, access string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if access != "" {
			req.Header.Set("Authorization", "Bearer "+access)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		var out struct{ Data map[string]any }
		if rec.Code == 200 {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out.Data
	}
	login := request("POST", "/api/v1/auth/google", `{"idToken":"verified-by-stub"}`, "", 200)
	access := login["accessToken"].(string)
	refresh := login["refreshToken"].(string)
	request("GET", "/api/v1/me", "", access, 200)
	rotated := request("POST", "/api/v1/auth/refresh", `{"refreshToken":"`+refresh+`"}`, "", 200)
	request("POST", "/api/v1/auth/refresh", `{"refreshToken":"`+refresh+`"}`, "", 401)
	request("POST", "/api/v1/auth/logout", `{"refreshToken":"`+rotated["refreshToken"].(string)+`"}`, access, 204)
	request("POST", "/api/v1/auth/refresh", `{"refreshToken":"`+rotated["refreshToken"].(string)+`"}`, "", 401)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET is_active=false WHERE google_sub='http-sub'`); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/v1/me", "", access, 401)
}
