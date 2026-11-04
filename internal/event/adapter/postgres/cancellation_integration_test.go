//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	auth "github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	eventhttp "github.com/qldapm-hcmut/ticket-system-uni/internal/event/adapter/http"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
	regpg "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/adapter/postgres"
	regapp "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
)

func TestEventCancellation(t *testing.T) {
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
	name := fmt.Sprintf("ticket_cancel_test_%d", time.Now().UnixNano())
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var s string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	schema, err := os.ReadFile("../../../../db/migrations/0001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(schema))
	user := func() string {
		return scalar(`INSERT INTO users(email,full_name) VALUES(gen_random_uuid()::text||'@hcmut.edu.vn','Test') RETURNING id::text`)
	}
	owner, outsider := user(), user()
	unit := scalar(`INSERT INTO units(code,name,type) VALUES('CANCEL','Cancel','FACULTY') RETURNING id::text`)
	exec(`INSERT INTO unit_memberships(user_id,unit_id,member_role) VALUES($1,$2,'ORGANIZER')`, owner, unit)
	svc := a.New(New(pool), kernel.SystemClock{})
	regs := regapp.New(regpg.New(pool))
	actor := a.Actor{ID: owner, IsOrganizer: true}
	event := func() string {
		return scalar(`INSERT INTO events(unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity,waitlist_limit,status) VALUES($1,$2,'Cancel test','Campus',now()+interval '3 days',now()+interval '4 days',now()-interval '1 day',now()+interval '2 days',1,10,'PUBLISHED') RETURNING id::text`, unit, owner)
	}
	seed := func(e string) {
		t.Helper()
		for i := 0; i < 2; i++ {
			if _, err := regs.Register(ctx, user(), e); err != nil {
				t.Fatal(err)
			}
		}
	}
	snapshot := func(e string) string {
		return scalar(`SELECT jsonb_build_object('event',to_jsonb(e),'registrations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM registrations r WHERE event_id=e.id),'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY id) FROM notifications n WHERE event_id=e.id),'audit',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM activity_logs l WHERE event_id=e.id))::text FROM events e WHERE id=$1`, e)
	}
	t.Run("HTTP success and policy", func(t *testing.T) {
		id := event()
		seed(id)
		exec(`UPDATE registrations SET checked_in_at=now(),checked_in_by=$2,checkin_method='CODE',cancellation_exception_until=now()+interval '3 days' WHERE event_id=$1 AND status='REGISTERED'`, id, owner)
		e := echo.New()
		eventhttp.New(svc, NewReader(pool, kernel.SystemClock{})).Register(e, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				c.Set("auth.profile", auth.Profile{ID: owner, Memberships: []auth.Membership{{}}})
				return next(c)
			}
		})
		req := func(body, version string, want int) *httptest.ResponseRecorder {
			t.Helper()
			r := httptest.NewRequest("POST", "/api/v1/events/"+id+"/cancel", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if version != "" {
				r.Header.Set("If-Match", version)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, r)
			if rec.Code != want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			return rec
		}
		before := snapshot(id)
		req(`{"reason":" "}`, "", 400)
		req(`{"reason":"Closed"}`, "bad", 400)
		req(`{"reason":"Closed"}`, "2", 409)
		if _, err := svc.Cancel(ctx, a.Actor{ID: outsider, IsOrganizer: true}, id, 0, "Closed"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
		if snapshot(id) != before {
			t.Fatal("invalid request mutated state")
		}
		rec := req(`{"reason":"Venue unavailable"}`, "1", 200)
		var body struct {
			Data map[string]any
			Meta struct{ AffectedCount int64 }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Meta.AffectedCount != 2 || body.Data["status"] != "CANCELLED" {
			t.Fatal(rec.Body.String())
		}
		if state := scalar(`SELECT status::text||':'||registered_count::text||':'||waitlist_count::text||':'||version::text FROM events WHERE id=$1`, id); state != "CANCELLED:0:0:2" {
			t.Fatal(state)
		}
		if n := scalar(`SELECT count(*)::text FROM registrations WHERE event_id=$1 AND status='CANCELLED_BY_EVENT' AND cancelled_by='ORGANIZER' AND cancelled_by_user=$2 AND cancellation_exception_until IS NULL`, id, owner); n != "2" {
			t.Fatal(n)
		}
		if n := scalar(`SELECT count(*)::text FROM registrations WHERE event_id=$1 AND checked_in_at IS NOT NULL`, id); n != "1" {
			t.Fatal("lost checkin history")
		}
		if n := scalar(`SELECT count(*)::text FROM notifications WHERE event_id=$1 AND type='EVENT_CANCELLED' AND body='Venue unavailable'`, id); n != "2" {
			t.Fatal(n)
		}
		if n := scalar(`SELECT count(*)::text FROM activity_logs WHERE event_id=$1 AND action='CANCEL_EVENT' AND metadata->>'affectedCount'='2'`, id); n != "1" {
			t.Fatal(n)
		}
		req(`{"reason":"Again"}`, "", 409)
		if _, err := regs.Register(ctx, user(), id); err == nil {
			t.Fatal("registration after cancellation succeeded")
		}
	})
	for _, table := range []string{"notifications", "activity_logs"} {
		t.Run("rollback "+table, func(t *testing.T) {
			id := event()
			seed(id)
			before := snapshot(id)
			exec(`CREATE FUNCTION fail_cancel() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'failure'; END $$; CREATE TRIGGER fail_cancel BEFORE INSERT ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION fail_cancel()`)
			result, err := svc.Cancel(ctx, actor, id, 1, "Closed")
			if err == nil || result.AffectedCount != 0 {
				t.Fatal("failed transaction returned success")
			}
			exec(`DROP TRIGGER fail_cancel ON ` + table + `; DROP FUNCTION fail_cancel()`)
			if snapshot(id) != before {
				t.Fatal("partial cancellation persisted")
			}
		})
	}
	t.Run("concurrent cancellation", func(t *testing.T) {
		id := event()
		seed(id)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := svc.Cancel(ctx, actor, id, 0, "Closed"); results <- err }()
		}
		wg.Wait()
		close(results)
		successes, conflicts := 0, 0
		for err := range results {
			if err == nil {
				successes++
			} else if errors.Is(err, domain.ErrState) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatal(successes, conflicts)
		}
		if n := scalar(`SELECT count(*)::text FROM notifications WHERE event_id=$1 AND type='EVENT_CANCELLED'`, id); n != "2" {
			t.Fatal("duplicate notifications", n)
		}
	})
	t.Run("empty and inactive scope", func(t *testing.T) {
		id := event()
		exec(`UPDATE units SET is_active=false WHERE id=$1`, unit)
		if _, err := svc.Cancel(ctx, actor, id, 0, "Closed"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
		exec(`UPDATE units SET is_active=true WHERE id=$1`, unit)
		result, err := svc.Cancel(ctx, actor, id, 0, "Closed")
		if err != nil || result.AffectedCount != 0 {
			t.Fatal(result, err)
		}
	})
}
