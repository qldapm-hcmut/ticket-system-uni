//go:build integration

package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Each run creates a separate database; never migrate/down the caller's database.
func TestInitialSchema(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is required (PostgreSQL 16 role with CREATEDB)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("ticket_schema_test_%d", time.Now().UnixNano())
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("cleanup database: %v", err)
		}
	}()
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = name
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	exec := func(sql string) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	up, down := read("migrations/0001_initial_schema.up.sql"), read("migrations/0001_initial_schema.down.sql")
	exec(up)
	exec(down)
	exec(up)
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("got %d tables, want 8", count)
	}
	exec(`INSERT INTO users (id,email,full_name,student_code) VALUES
 ('00000000-0000-0000-0000-000000000001','student@hcmut.edu.vn','Student','2210001')`)
	exec(`INSERT INTO units (id,code,name,type) VALUES ('00000000-0000-0000-0000-000000000002','TEST','Test','FACULTY')`)
	exec(`INSERT INTO events (id,unit_id,created_by,title,location,start_at,end_at,registration_opens_at,registration_closes_at,capacity)
 VALUES ('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','Test','Room',now()+interval '2 days',now()+interval '3 days',now(),now()+interval '1 day',1)`)
	exec(`INSERT INTO registrations (event_id,student_id,status,checkin_code) VALUES ('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000001','REGISTERED','ABCDEFGH')`)
	tests := []struct{ name, sql, constraint string }{
		{"capacity", `UPDATE events SET registered_count=2`, "chk_capacity"},
		{"self approval", `UPDATE events SET approved_by=created_by`, "chk_no_self_approve"},
		{"duplicate active", `INSERT INTO registrations (event_id,student_id,status,checkin_code) SELECT event_id,student_id,'WAITLISTED','12345678' FROM registrations`, "uq_active_registration"},
		{"waitlist exception", `UPDATE registrations SET status='WAITLISTED',cancellation_exception_until=now()`, "chk_cancel_exception"},
		{"self checkin", `UPDATE registrations SET checked_in_at=now(),checkin_method='QR',checked_in_by=student_id`, "chk_no_self_checkin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := conn.Exec(ctx, tt.sql)
			pgErr, ok := err.(*pgconn.PgError)
			if !ok || pgErr.ConstraintName != tt.constraint {
				t.Fatalf("want constraint %s, got %v", tt.constraint, err)
			}
		})
	}
}
