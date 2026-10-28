//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
)

func TestPersistence(t *testing.T) {
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
	name := fmt.Sprintf("ticket_auth_test_%d", time.Now().UnixNano())
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
	sql, err := os.ReadFile("../../../../db/migrations/0001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	uow := NewUnitOfWork(pool)
	t.Run("login use cases", func(t *testing.T) { testLoginUseCases(t, pool, uow) })
	now := time.Now().UTC()
	var owner application.User
	err = uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
		var err error
		owner, err = r.CreateUser(ctx, "test@hcmut.edu.vn", "Test", "sub-test")
		if err != nil {
			return err
		}
		if owner.IsAdmin || owner.StudentCode != "" {
			return errors.New("unexpected privileges")
		}
		_, err = r.CreateSession(ctx, owner.ID, "hash-original", now.Add(time.Hour))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("rollback")
	err = uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
		if _, err := r.ConsumeSession(ctx, "hash-original", now); err != nil {
			return err
		}
		if _, err := r.CreateSession(ctx, owner.ID, "hash-rolled-back", now.Add(time.Hour)); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		hash    string
		expires time.Time
		revoke  bool
	}{
		{"hash-expired", now, false},
		{"hash-revoked", now.Add(time.Hour), true},
	} {
		err := uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
			if _, err := r.CreateSession(ctx, owner.ID, tc.hash, tc.expires); err != nil {
				return err
			}
			if tc.revoke {
				return r.RevokeSession(ctx, owner.ID, tc.hash, now)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		err = uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
			_, err := r.ConsumeSession(ctx, tc.hash, now)
			return err
		})
		if !errors.Is(err, application.ErrNotFound) {
			t.Fatalf("%s should be rejected: %v", tc.hash, err)
		}
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
				_, err := r.ConsumeSession(ctx, "hash-original", now)
				return err
			})
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, application.ErrNotFound) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("consume successes: %d", successes.Load())
	}
	err = uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
		_, err := r.ConsumeSession(ctx, "hash-rolled-back", now)
		return err
	})
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("replacement persisted after rollback: %v", err)
	}
	err = uow.Do(ctx, func(ctx context.Context, r application.Repository) error {
		_, err := r.CreateUser(ctx, "test@hcmut.edu.vn", "Other", "different-sub")
		return err
	})
	if !errors.Is(err, application.ErrIdentityConflict) {
		t.Fatalf("expected identity conflict: %v", err)
	}
}
