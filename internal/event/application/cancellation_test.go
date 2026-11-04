package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

type cancelRepo struct {
	EventRepository
	event     domain.Event
	allowed   bool
	cancelled bool
	locked    bool
	err       error
}

func (r *cancelRepo) Get(_ context.Context, _ string, lock bool) (domain.Event, error) {
	r.locked = lock
	return r.event, r.err
}
func (r *cancelRepo) IsOrganizer(context.Context, string, string) (bool, error) {
	return r.allowed, nil
}
func (r *cancelRepo) Cancel(_ context.Context, e domain.Event, _, _ string) (domain.Event, int64, error) {
	r.cancelled = true
	e.Status = "CANCELLED"
	return e, 2, nil
}

type cancelUOW struct {
	repo  *cancelRepo
	calls int
}

func (u *cancelUOW) Do(ctx context.Context, fn func(context.Context, EventRepository) error) error {
	u.calls++
	return fn(ctx, u.repo)
}
func TestCancelPolicy(t *testing.T) {
	for _, tc := range []struct {
		name           string
		actor          Actor
		status, reason string
		version        int32
		allowed        bool
		want           error
	}{
		{"success", Actor{ID: "owner", IsOrganizer: true}, "PUBLISHED", "Unavailable", 2, true, nil},
		{"optional version", Actor{IsOrganizer: true}, "PUBLISHED", "Unavailable", 0, true, nil},
		{"admin only", Actor{IsAdmin: true}, "PUBLISHED", "Unavailable", 2, true, domain.ErrForbidden},
		{"foreign unit", Actor{IsOrganizer: true}, "PUBLISHED", "Unavailable", 2, false, domain.ErrNotFound},
		{"stale", Actor{IsOrganizer: true}, "PUBLISHED", "Unavailable", 1, true, domain.ErrVersion},
		{"already cancelled", Actor{IsOrganizer: true}, "CANCELLED", "Unavailable", 2, true, domain.ErrState},
		{"draft", Actor{IsOrganizer: true}, "DRAFT", "Unavailable", 2, true, domain.ErrState},
		{"blank reason", Actor{IsOrganizer: true}, "PUBLISHED", " \n", 2, true, domain.ErrValidation},
		{"too long", Actor{IsOrganizer: true}, "PUBLISHED", strings.Repeat("á", 501), 2, true, domain.ErrValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &cancelRepo{event: domain.Event{Status: tc.status, Version: 2}, allowed: tc.allowed}
			u := &cancelUOW{repo: r}
			out, err := New(u, nil).Cancel(context.Background(), tc.actor, "event", tc.version, tc.reason)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if tc.want == nil {
				if !r.locked || !r.cancelled || out.AffectedCount != 2 || out.Event.Status != "CANCELLED" {
					t.Fatal(out, r)
				}
			} else if r.cancelled {
				t.Fatal("invalid request reached mutation")
			}
		})
	}
}
