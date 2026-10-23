package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestHealthAndErrors(t *testing.T) {
	for _, tt := range []struct {
		name, method, path string
		dbErr              error
		status             int
		code               string
	}{
		{"live", "GET", "/health/live", errors.New("private DB error"), 200, ""},
		{"ready", "GET", "/health/ready", nil, 200, ""},
		{"unavailable", "GET", "/health/ready", errors.New("private DB error"), 503, "SERVICE_UNAVAILABLE"},
		{"not found", "GET", "/missing", nil, 404, "NOT_FOUND"},
		{"wrong method", "POST", "/health/live", nil, 405, "METHOD_NOT_ALLOWED"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := New(fakeDB{tt.dbErr})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != tt.code {
				t.Fatalf("error code = %s", body.Error.Code)
			}
			if rec.Header().Get("X-Request-Id") == "" {
				t.Fatal("missing request ID")
			}
		})
	}
}
