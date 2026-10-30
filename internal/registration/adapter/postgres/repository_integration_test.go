//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	eventpg "github.com/qldapm-hcmut/ticket-system-uni/internal/event/adapter/postgres"
	eventapp "github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
)

func TestRegistrationPersistence(t *testing.T) {
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
	name := fmt.Sprintf("ticket_registration_test_%d", time.Now().UnixNano())
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
	schema, err := os.ReadFile("../../../../db/migrations/0001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	service := a.New(New(pool))
	user := func() string {
		t.Helper()
		var id string
		err := pool.QueryRow(ctx, `INSERT INTO users(email,full_name) VALUES (gen_random_uuid()::text || '@hcmut.edu.vn','Test') RETURNING id::text`).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner := user()
	var unit string
	if err := pool.QueryRow(ctx, `INSERT INTO units(code,name,type) VALUES ('TEST','Test','FACULTY') RETURNING id::text`).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	event := func(capacity, limit int) string {
		t.Helper()
		var id string
		err := pool.QueryRow(ctx, `INSERT INTO events(unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity,waitlist_limit,status) VALUES ($1,$2,'Test','Campus',now()+interval '3 days',now()+interval '4 days',now()-interval '1 day',now()+interval '2 days',$3,$4,'PUBLISHED') RETURNING id::text`, unit, owner, capacity, limit).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	counters := func(id string, registered, waiting int) {
		t.Helper()
		var r, w, actualR, actualW int
		if err := pool.QueryRow(ctx, `SELECT registered_count,waitlist_count,(SELECT count(*) FROM registrations WHERE event_id=events.id AND status='REGISTERED'),(SELECT count(*) FROM registrations WHERE event_id=events.id AND status='WAITLISTED') FROM events WHERE id=$1`, id).Scan(&r, &w, &actualR, &actualW); err != nil {
			t.Fatal(err)
		}
		if r != registered || w != waiting || r != actualR || w != actualW {
			t.Fatalf("counters=(%d,%d), rows=(%d,%d), want=(%d,%d)", r, w, actualR, actualW, registered, waiting)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO unit_memberships(user_id,unit_id,member_role) VALUES ($1,$2,'ORGANIZER')`, owner, unit); err != nil {
		t.Fatal(err)
	}
	es := eventapp.New(eventpg.New(pool), kernel.SystemClock{})
	patch := func(e string, capacity, version int32) (domain.Event, error) {
		return es.Patch(ctx, eventapp.Actor{ID: owner, IsOrganizer: true}, e, version, func(f domain.Fields) (domain.Fields, error) { f.Capacity = capacity; return f, nil })
	}
	seed := func(e string, n int) []a.Registration {
		t.Helper()
		out := make([]a.Registration, n)
		for i := range out {
			var err error
			out[i], err = service.Register(ctx, user(), e)
			if err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	checkQueue := func(e string) {
		t.Helper()
		var violations int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM registrations w JOIN registrations p ON p.event_id=w.event_id WHERE w.event_id=$1 AND w.status='WAITLISTED' AND p.status='REGISTERED' AND p.queue_seq>w.queue_seq`, e).Scan(&violations); err != nil {
			t.Fatal(err)
		}
		if violations != 0 {
			t.Fatalf("FIFO violations=%d", violations)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT user_id,type FROM notifications WHERE event_id=$1 GROUP BY user_id,type HAVING count(*)>1) duplicates`, e).Scan(&violations); err != nil {
			t.Fatal(err)
		}
		if violations != 0 {
			t.Fatalf("duplicate notifications=%d", violations)
		}
		var promoted, notified int
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM registrations WHERE event_id=$1 AND promoted_at IS NOT NULL),(SELECT count(*) FROM notifications WHERE event_id=$1 AND type='PROMOTED')`, e).Scan(&promoted, &notified); err != nil {
			t.Fatal(err)
		}
		if promoted != notified {
			t.Fatalf("promotions=%d notifications=%d", promoted, notified)
		}
	}
	// A shared start barrier makes all operations contend rather than run serially.
	concurrent := func(tasks ...func() error) []error {
		start := make(chan struct{})
		results := make(chan error, len(tasks))
		var wg sync.WaitGroup
		for _, task := range tasks {
			wg.Add(1)
			go func(task func() error) { defer wg.Done(); <-start; results <- task() }(task)
		}
		close(start)
		wg.Wait()
		close(results)
		out := make([]error, 0, len(tasks))
		for err := range results {
			out = append(out, err)
		}
		return out
	}
	t.Run("capacity exceeds queue and lower boundary", func(t *testing.T) {
		e := event(1, 5)
		seed(e, 3)
		result, err := patch(e, 10, 1)
		if err != nil {
			t.Fatal(err)
		}
		if result.PromotedCount != 2 {
			t.Fatalf("promoted=%d", result.PromotedCount)
		}
		counters(e, 3, 0)
		result, err = patch(e, 3, 2)
		if err != nil {
			t.Fatal(err)
		}
		if result.PromotedCount != 0 {
			t.Fatal("decrease promoted waitlist")
		}
		if _, err = patch(e, 2, 3); !errors.Is(err, domain.ErrCapacity) {
			t.Fatalf("expected capacity error: %v", err)
		}
		counters(e, 3, 0)
		checkQueue(e)
	})
	t.Run("started event cannot promote", func(t *testing.T) {
		e := event(1, 5)
		regs := seed(e, 2)
		if _, err := pool.Exec(ctx, `UPDATE events SET start_at=now()-interval '1 hour',end_at=now()+interval '1 hour',registration_opens_at=now()-interval '3 days',registration_closes_at=now()-interval '2 hours' WHERE id=$1`, e); err != nil {
			t.Fatal(err)
		}
		if _, err := patch(e, 2, 1); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected past-start validation: %v", err)
		}
		if _, err := service.Cancel(ctx, regs[0].StudentID, regs[0].ID); !errors.Is(err, a.ErrCancelDeadline) {
			t.Fatalf("expected deadline: %v", err)
		}
		counters(e, 1, 1)
		checkQueue(e)
	})
	t.Run("concurrent patches same version", func(t *testing.T) {
		e := event(1, 10)
		seed(e, 8)
		results := concurrent(func() error { _, err := patch(e, 3, 1); return err }, func() error { _, err := patch(e, 3, 1); return err })
		success, conflict := 0, 0
		for _, err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, domain.ErrVersion) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success=%d conflict=%d", success, conflict)
		}
		counters(e, 3, 5)
		checkQueue(e)
	})
	t.Run("concurrent cancellations", func(t *testing.T) {
		e := event(4, 20)
		regs := seed(e, 14)
		tasks := make([]func() error, 4)
		for i := range tasks {
			r := regs[i]
			tasks[i] = func() error { _, err := service.Cancel(ctx, r.StudentID, r.ID); return err }
		}
		for _, err := range concurrent(tasks...) {
			if err != nil {
				t.Fatal(err)
			}
		}
		counters(e, 4, 6)
		checkQueue(e)
	})
	t.Run("capacity registration cancellation race", func(t *testing.T) {
		for iteration := 0; iteration < 5; iteration++ {
			e := event(2, 30)
			regs := seed(e, 12)
			newStudent := user()
			tasks := []func() error{
				func() error { _, err := patch(e, 5, 1); return err },
				func() error { _, err := service.Register(ctx, newStudent, e); return err },
				func() error { _, err := service.Cancel(ctx, regs[0].StudentID, regs[0].ID); return err },
				func() error { _, err := service.Cancel(ctx, regs[1].StudentID, regs[1].ID); return err },
			}
			for _, err := range concurrent(tasks...) {
				if err != nil {
					t.Fatal(err)
				}
			}
			counters(e, 5, 6)
			checkQueue(e)
		}
	})
	t.Run("PROMOTED notification failure rolls back cancellation", func(t *testing.T) {
		e := event(1, 3)
		regs := seed(e, 3)
		if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_promoted() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.type='PROMOTED' THEN RAISE EXCEPTION 'injected promotion failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_promoted BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION reject_promoted()`); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := pool.Exec(ctx, `DROP TRIGGER reject_promoted ON notifications; DROP FUNCTION reject_promoted()`); err != nil {
				t.Error(err)
			}
		}()
		if _, err := service.Cancel(ctx, regs[0].StudentID, regs[0].ID); err == nil {
			t.Fatal("expected promotion notification failure")
		}
		counters(e, 1, 2)
		checkQueue(e)
		var status string
		var cancelled, promoted *time.Time
		if err := pool.QueryRow(ctx, `SELECT status,cancelled_at,promoted_at FROM registrations WHERE id=$1`, regs[0].ID).Scan(&status, &cancelled, &promoted); err != nil {
			t.Fatal(err)
		}
		if status != "REGISTERED" || cancelled != nil || promoted != nil {
			t.Fatalf("cancellation persisted: %s %v %v", status, cancelled, promoted)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id=$1 AND promoted_at IS NOT NULL`, e).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("promotion timestamp persisted")
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE event_id=$1 AND type IN ('PROMOTED','REG_CANCELLED')`, e).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("transaction notifications persisted")
		}
	})
	t.Run("capacity promotion FIFO and notification rollback", func(t *testing.T) {
		e := event(1, 5)
		if _, err := pool.Exec(ctx, `INSERT INTO unit_memberships(user_id,unit_id,member_role) VALUES ($1,$2,'ORGANIZER') ON CONFLICT DO NOTHING`, owner, unit); err != nil {
			t.Fatal(err)
		}
		students := []string{user(), user(), user(), user()}
		for _, s := range students {
			if _, err := service.Register(ctx, s, e); err != nil {
				t.Fatal(err)
			}
		}
		es := eventapp.New(eventpg.New(pool), kernel.SystemClock{})
		patch := func(capacity int32, version int32) error {
			_, err := es.Patch(ctx, eventapp.Actor{ID: owner, IsOrganizer: true}, e, version, func(f domain.Fields) (domain.Fields, error) { f.Capacity = capacity; return f, nil })
			return err
		}
		if err := patch(3, 1); err != nil {
			t.Fatal(err)
		}
		counters(e, 3, 1)
		for i, s := range students {
			var status string
			if err := pool.QueryRow(ctx, `SELECT status FROM registrations WHERE event_id=$1 AND student_id=$2`, e, s).Scan(&status); err != nil {
				t.Fatal(err)
			}
			want := "REGISTERED"
			if i == 3 {
				want = "WAITLISTED"
			}
			if status != want {
				t.Fatalf("student %d: %s", i, status)
			}
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE event_id=$1 AND type='PROMOTED'`, e).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Fatalf("promoted notifications=%d", n)
		}
		if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_notification() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected notification failure'; END $$; CREATE TRIGGER reject_notification BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION reject_notification()`); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, `DROP TRIGGER reject_notification ON notifications; DROP FUNCTION reject_notification()`)
		if err := patch(4, 2); err == nil {
			t.Fatal("expected notification failure")
		}
		counters(e, 3, 1)
		var capacity, version int
		if err := pool.QueryRow(ctx, `SELECT capacity,version FROM events WHERE id=$1`, e).Scan(&capacity, &version); err != nil {
			t.Fatal(err)
		}
		if capacity != 3 || version != 2 {
			t.Fatalf("update not rolled back: %d %d", capacity, version)
		}
		fresh := event(1, 1)
		if _, err := service.Register(ctx, user(), fresh); err == nil {
			t.Fatal("expected registration notification failure")
		}
		counters(fresh, 0, 0)
		var regID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM registrations WHERE event_id=$1 AND student_id=$2`, e, students[0]).Scan(&regID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Cancel(ctx, students[0], regID); err == nil {
			t.Fatal("expected cancellation notification failure")
		}
		counters(e, 3, 1)
	})
	t.Run("FIFO promotion and cancellation", func(t *testing.T) {
		e := event(1, 3)
		students := []string{user(), user(), user()}
		regs := make([]a.Registration, 3)
		for i, s := range students {
			var err error
			regs[i], err = service.Register(ctx, s, e)
			if err != nil {
				t.Fatal(err)
			}
		}
		if regs[0].Status != "REGISTERED" || regs[1].Status != "WAITLISTED" || regs[2].Status != "WAITLISTED" || regs[1].QueueSeq >= regs[2].QueueSeq {
			t.Fatalf("unexpected queue: %+v", regs)
		}
		counters(e, 1, 2)
		for i := 0; i < 2; i++ {
			result, err := service.Cancel(ctx, students[i], regs[i].ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Registration.Status != "CANCELLED" || result.Promoted == nil || result.Promoted.ID != regs[i+1].ID || result.Promoted.Status != "REGISTERED" {
				t.Fatalf("unexpected promotion: %+v", result)
			}
			counters(e, 1, 1-i)
		}
	})
	t.Run("waitlist cancellation does not promote", func(t *testing.T) {
		e := event(1, 2)
		s1, s2 := user(), user()
		if _, err := service.Register(ctx, s1, e); err != nil {
			t.Fatal(err)
		}
		r, err := service.Register(ctx, s2, e)
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Cancel(ctx, s2, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if result.Promoted != nil {
			t.Fatal("waitlist cancellation promoted someone")
		}
		counters(e, 1, 0)
	})
	t.Run("concurrent registrations respect capacity and waitlist limit", func(t *testing.T) {
		e := event(3, 5)
		students := make([]string, 24)
		for i := range students {
			students[i] = user()
		}
		results := make(chan error, len(students))
		var wg sync.WaitGroup
		for _, s := range students {
			wg.Add(1)
			go func(s string) { defer wg.Done(); _, err := service.Register(ctx, s, e); results <- err }(s)
		}
		wg.Wait()
		close(results)
		success, full := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, a.ErrFull) {
				full++
			} else {
				t.Error(err)
			}
		}
		if success != 8 || full != 16 {
			t.Fatalf("success=%d full=%d", success, full)
		}
		counters(e, 3, 5)
	})
	t.Run("concurrent duplicate registration", func(t *testing.T) {
		e, s := event(2, 2), user()
		results := make(chan error, 12)
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := service.Register(ctx, s, e); results <- err }()
		}
		wg.Wait()
		close(results)
		success, duplicate := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, a.ErrAlreadyActive) {
				duplicate++
			} else {
				t.Error(err)
			}
		}
		if success != 1 || duplicate != 11 {
			t.Fatalf("success=%d duplicate=%d", success, duplicate)
		}
		counters(e, 1, 0)
	})
}
