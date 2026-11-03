//go:build integration

package postgres

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	reporthttp "github.com/qldapm-hcmut/ticket-system-uni/internal/report/adapter/http"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/report/application"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func TestReportFlow(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL required")
	}
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("ticket_report_test_%d", time.Now().UnixNano())
	ident := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+ident+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema, err := os.ReadFile("../../../../db/migrations/0001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(string(schema))
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	user := func() string {
		return scalar(`INSERT INTO users(email,full_name) VALUES(gen_random_uuid()::text||'@hcmut.edu.vn','Nguyễn, An') RETURNING id::text`)
	}
	owner, schoolAdmin, outsider := user(), user(), user()
	exec(`UPDATE users SET is_admin=true WHERE id=$1`, schoolAdmin)
	unit := scalar(`INSERT INTO units(code,name,type) VALUES('CSE','CSE','FACULTY') RETURNING id::text`)
	exec(`INSERT INTO unit_memberships(user_id,unit_id,member_role) VALUES($1,$2,'ORGANIZER')`, owner, unit)
	event := scalar(`INSERT INTO events(unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity,status) VALUES($1,$2,'Report','Campus',now()+interval '1 day',now()+interval '2 days',now()-interval '1 day',now()+interval '1 hour',600,'PUBLISHED') RETURNING id::text`, unit, owner)
	// 500 rows cover CSV quoting, formula safety, aggregation and target size.
	exec(`INSERT INTO users(email,full_name,student_code) SELECT 'report'||n||'@hcmut.edu.vn',CASE WHEN n=1 THEN '=1+1' ELSE 'Nguyễn, An' END,'S'||n FROM generate_series(1,500) n`)
	exec(`INSERT INTO registrations(event_id,student_id,status,checkin_code,promoted_at,checked_in_at,checked_in_by,checkin_method)
 SELECT $1,id,'REGISTERED',lpad(row_number() OVER(ORDER BY id)::text,8,'0'),now(),now(),$2,'QR' FROM users WHERE student_code IS NOT NULL`, event, owner)
	waiting := user()
	exec(`INSERT INTO registrations(event_id,student_id,status,checkin_code) VALUES($1,$2,'WAITLISTED','WAIT0001')`, event, waiting)
	c := &clock{time.Now()}
	store := New(pool, c)
	svc := a.New(store)
	summary, err := svc.Summary(ctx, owner, event)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Registered != 500 || summary.Waitlisted != 1 || summary.CheckedIn != 500 || summary.Promoted != 500 || summary.AttendanceRate != 1 || summary.ByMethod["QR"] != 500 {
		t.Fatalf("summary: %+v", summary)
	}
	var startAt time.Time
	if err := pool.QueryRow(ctx, `SELECT start_at FROM events WHERE id=$1`, event).Scan(&startAt); err != nil {
		t.Fatal(err)
	}
	c.now = startAt.Add(-time.Nanosecond)
	if before, err := svc.Summary(ctx, owner, event); err != nil || before.Waitlisted != 1 {
		t.Fatalf("before start: %+v %v", before, err)
	}
	c.now = startAt
	summary, err = svc.Summary(ctx, schoolAdmin, event)
	if err != nil || summary.Waitlisted != 0 {
		t.Fatalf("expired waitlist: %+v %v", summary, err)
	}
	for _, actor := range []string{outsider, "00000000-0000-0000-0000-000000000000"} {
		if _, err := svc.Summary(ctx, actor, event); !errors.Is(err, a.ErrNotFound) {
			t.Fatalf("scope: %v", err)
		}
		if _, err := svc.Export(ctx, actor, event, "attendance", "csv"); !errors.Is(err, a.ErrNotFound) {
			t.Fatalf("export scope: %v", err)
		}
	}
	start := time.Now()
	data, err := svc.Export(ctx, owner, event, "attendance", "csv")
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatal("export exceeded 30 seconds")
	}
	if !strings.HasPrefix(string(data), "\xef\xbb\xbf") {
		t.Fatal("missing BOM")
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(records) != 501 {
		t.Fatalf("CSV rows=%d err=%v", len(records), err)
	}
	if !strings.Contains(string(data), "'=1+1") || !strings.Contains(string(data), `"Nguyễn, An"`) {
		t.Fatal("CSV cell safety/quoting")
	}
	if scalar(`SELECT count(*)::text FROM activity_logs WHERE event_id=$1 AND action='EXPORT_DATA' AND metadata->>'rowCount'='500'`, event) != "1" {
		t.Fatal("audit missing")
	}
	data, err = svc.Export(ctx, schoolAdmin, event, "registrations", "")
	if err != nil {
		t.Fatal(err)
	}
	records, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(records) != 502 {
		t.Fatal("registration rows", len(records), err)
	}
	// Audit failure must prevent any file bytes escaping.
	exec(`CREATE FUNCTION fail_export() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER fail_export BEFORE INSERT ON activity_logs FOR EACH ROW EXECUTE FUNCTION fail_export()`)
	if data, err := svc.Export(ctx, owner, event, "attendance", "csv"); err == nil || data != nil {
		t.Fatal("export escaped failed audit")
	}
	exec(`DROP TRIGGER fail_export ON activity_logs`)
	e := echo.New()
	reporthttp.New(svc).Register(e, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { c.Set("auth.profile", auth.Profile{ID: owner}); return next(c) }
	})
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/reports/summary", 200}, {"/exports/attendance?format=csv", 200}, {"/exports/attendance?format=xlsx", 400}, {"/exports/invalid", 400},
	} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/events/"+event+tc.path, nil))
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
		if tc.path == "/exports/attendance?format=csv" && rec.Header().Get("Content-Disposition") != `attachment; filename="attendance.csv"` {
			t.Fatal("attachment header")
		}
	}
	// An inactive unit removes organizer access, but not school-admin access.
	exec(`UPDATE units SET is_active=false WHERE id=$1`, unit)
	if _, err := svc.Summary(ctx, owner, event); !errors.Is(err, a.ErrNotFound) {
		t.Fatal("inactive unit scope", err)
	}
	if data, err := svc.Export(ctx, owner, event, "registrations", "csv"); !errors.Is(err, a.ErrNotFound) || data != nil {
		t.Fatal("inactive unit export", err)
	}
	if _, err := svc.Summary(ctx, schoolAdmin, event); err != nil {
		t.Fatal("admin access to inactive unit", err)
	}
	exec(`UPDATE units SET is_active=true WHERE id=$1`, unit)
	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if _, err := svc.Summary(ctx, owner, id); !errors.Is(err, a.ErrNotFound) {
			t.Fatal("invalid/missing event", err)
		}
	}
	// Undo must immediately remove a row from attendance and its method count.
	exec(`UPDATE registrations SET checked_in_at=NULL, checked_in_by=NULL, checkin_method=NULL WHERE event_id=$1 AND student_id=(SELECT student_id FROM registrations WHERE event_id=$1 AND checked_in_at IS NOT NULL LIMIT 1)`, event)
	summary, err = svc.Summary(ctx, owner, event)
	if err != nil || summary.CheckedIn != 499 || summary.ByMethod["QR"] != 499 || summary.AttendanceRate != 499.0/500 {
		t.Fatalf("after undo: %+v %v", summary, err)
	}
	data, err = svc.Export(ctx, owner, event, "attendance", "csv")
	if err != nil {
		t.Fatal(err)
	}
	records, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(records) != 500 {
		t.Fatalf("attendance after undo: %d %v", len(records), err)
	}
	exec(`UPDATE registrations SET checkin_method='CODE' WHERE event_id=$1 AND student_id=(SELECT student_id FROM registrations WHERE event_id=$1 AND checkin_method='QR' LIMIT 1)`, event)
	exec(`UPDATE registrations SET checkin_method='MANUAL', checkin_note='Checked by organizer' WHERE event_id=$1 AND student_id=(SELECT student_id FROM registrations WHERE event_id=$1 AND checkin_method='QR' LIMIT 1)`, event)
	exec(`UPDATE registrations SET status='CANCELLED', cancelled_at=now() WHERE event_id=$1 AND student_id=$2`, event, waiting)
	summary, err = svc.Summary(ctx, owner, event)
	if err != nil || summary.Cancelled != 1 || summary.ByMethod["QR"] != 497 || summary.ByMethod["CODE"] != 1 || summary.ByMethod["MANUAL"] != 1 {
		t.Fatalf("method/cancellation counts: %+v %v", summary, err)
	}
	// Empty events must return zero attendance rather than NaN/Inf.
	emptyEvent := scalar(`INSERT INTO events(unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity,status) VALUES($1,$2,'Empty','Campus',now()+interval '1 day',now()+interval '2 days',now()-interval '1 day',now()+interval '1 hour',10,'PUBLISHED') RETURNING id::text`, unit, owner)
	emptySummary, err := svc.Summary(ctx, owner, emptyEvent)
	if err != nil || emptySummary.Registered != 0 || emptySummary.AttendanceRate != 0 || len(emptySummary.ByMethod) != 3 {
		t.Fatalf("empty summary: %+v %v", emptySummary, err)
	}
	exec(`UPDATE users SET is_active=false WHERE id=$1`, owner)
	if _, err := svc.Summary(ctx, owner, event); !errors.Is(err, a.ErrNotFound) {
		t.Fatal("inactive actor", err)
	}
}
