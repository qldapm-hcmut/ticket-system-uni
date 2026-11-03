package application

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	calls              int
	actor, event, kind string
	err                error
}

func (f *fakeStore) Summary(_ context.Context, actor, event string) (Summary, error) {
	f.calls++
	f.actor, f.event = actor, event
	return Summary{Registered: 3}, f.err
}
func (f *fakeStore) Export(_ context.Context, actor, event, kind string) ([]byte, error) {
	f.calls++
	f.actor, f.event, f.kind = actor, event, kind
	return []byte("csv"), f.err
}
func TestExportOptions(t *testing.T) {
	for _, tc := range []struct {
		kind, format string
		valid        bool
	}{
		{"registrations", "", true}, {"registrations", "csv", true}, {"attendance", "csv", true},
		{"attendance", "xlsx", false}, {"invalid", "csv", false}, {"", "", false}, {"attendance", "CSV", false},
	} {
		t.Run(tc.kind+"/"+tc.format, func(t *testing.T) {
			f := &fakeStore{}
			data, err := New(f).Export(context.Background(), "actor", "event", tc.kind, tc.format)
			if !tc.valid {
				if !errors.Is(err, ErrValidation) || f.calls != 0 || data != nil {
					t.Fatalf("invalid options reached store: %q %v %+v", data, err, f)
				}
				return
			}
			if err != nil || string(data) != "csv" || f.calls != 1 || f.actor != "actor" || f.event != "event" || f.kind != tc.kind {
				t.Fatalf("export: %q %v %+v", data, err, f)
			}
		})
	}
}
func TestServicePropagatesStoreErrors(t *testing.T) {
	sentinel := errors.New("store unavailable")
	f := &fakeStore{err: sentinel}
	s := New(f)
	if _, err := s.Summary(context.Background(), "actor", "event"); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if f.actor != "actor" || f.event != "event" {
		t.Fatalf("scope lost: %+v", f)
	}
	if _, err := s.Export(context.Background(), "actor", "event", "attendance", ""); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
