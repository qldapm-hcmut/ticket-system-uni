//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLifecycleJobs(t *testing.T) {
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
	name := fmt.Sprintf("ticket_lifecycle_test_%d", time.Now().UnixNano())
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
	owner := scalar(`INSERT INTO users(email,full_name) VALUES('worker-test@hcmut.edu.vn','Worker') RETURNING id::text`)
	unit := scalar(`INSERT INTO units(code,name,type) VALUES('WORKER','Worker','FACULTY') RETURNING id::text`)
	event := func(start, end, status string) string {
		return scalar(`INSERT INTO events(unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity,status,waitlist_count) VALUES($1,$2,'Worker','Campus',now()+$3::interval,now()+$4::interval,now()-interval '10 days',now()-interval '9 days',1,$5,1) RETURNING id::text`, unit, owner, start, end, status)
	}
	waiting := func(id string) {
		exec(`INSERT INTO registrations(event_id,student_id,status,checkin_code) VALUES($1,$2,'WAITLISTED','WAIT0001')`, id, owner)
	}
	store := New(pool)
	future := event("1 day", "2 days", "PUBLISHED")
	waiting(future)
	started := event("-1 hour", "1 hour", "PUBLISHED")
	waiting(started)
	ended := event("-2 hours", "-1 hour", "PUBLISHED")
	waiting(ended)
	cancelled := event("-2 hours", "-1 hour", "CANCELLED")
	waiting(cancelled)
	if n, err := store.ExpireWaitlists(ctx); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	if n, err := store.ExpireWaitlists(ctx); err != nil || n != 0 {
		t.Fatal("not idempotent", n, err)
	}
	if s := scalar(`SELECT status::text FROM registrations WHERE event_id=$1`, future); s != "WAITLISTED" {
		t.Fatal(s)
	}
	for _, id := range []string{started, ended, cancelled} {
		if s := scalar(`SELECT status::text FROM registrations WHERE event_id=$1`, id); s != "EXPIRED" {
			t.Fatal(s)
		}
		if s := scalar(`SELECT waitlist_count::text FROM events WHERE id=$1`, id); s != "0" {
			t.Fatal(s)
		}
	}
	if n, err := store.CompleteEvents(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := store.CompleteEvents(ctx); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if s := scalar(`SELECT status::text||':'||version::text FROM events WHERE id=$1`, ended); s != "COMPLETED:2" {
		t.Fatal(s)
	}
	if s := scalar(`SELECT status::text FROM events WHERE id=$1`, cancelled); s != "CANCELLED" {
		t.Fatal(s)
	}
	// Failed event update must roll back registration expiration in the same batch.
	rollback := event("-2 hours", "-1 hour", "PUBLISHED")
	waiting(rollback)
	exec(`CREATE FUNCTION fail_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$; CREATE TRIGGER fail_lifecycle BEFORE UPDATE ON events FOR EACH ROW EXECUTE FUNCTION fail_lifecycle()`)
	if n, err := store.CompleteEvents(ctx); err == nil || n != 0 {
		t.Fatal("expected failure", n, err)
	}
	exec(`DROP TRIGGER fail_lifecycle ON events; DROP FUNCTION fail_lifecycle()`)
	if s := scalar(`SELECT status::text FROM registrations WHERE event_id=$1`, rollback); s != "WAITLISTED" {
		t.Fatal("partial commit", s)
	}
	if s := scalar(`SELECT status::text||':'||waitlist_count::text FROM events WHERE id=$1`, rollback); s != "PUBLISHED:1" {
		t.Fatal(s)
	}
	// Locked events are skipped, then processed by the next run.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM events WHERE id=$1 FOR UPDATE`, rollback); err != nil {
		t.Fatal(err)
	}
	if n, err := store.CompleteEvents(ctx); err != nil || n != 0 {
		t.Fatal("locked event not skipped", n, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	// Multiple replicas and more than one batch: every event transitions once.
	exec(`INSERT INTO events(unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity,status) SELECT $1,$2,'Batch','Campus',now()-interval '2 hours',now()-interval '1 hour',now()-interval '10 days',now()-interval '9 days',1,'PUBLISHED' FROM generate_series(1,205)`, unit, owner)
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n, err := store.CompleteEvents(ctx); results <- n; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	for n := range results {
		total += n
	}
	if total != 206 {
		t.Fatal("replica count", total)
	}
	if s := scalar(`SELECT count(*)::text FROM events WHERE status='COMPLETED' AND version=2`); s != "207" {
		t.Fatal(s)
	}
	if s := scalar(`SELECT count(*)::text FROM notifications`); s != "0" {
		t.Fatal("unexpected notifications", s)
	}
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.ExpireWaitlists(cancelledCtx); err == nil {
		t.Fatal("cancelled context ignored")
	}
}
