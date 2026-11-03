package application

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	calls int
	actor string
	err   error
}

func (f *fakeStore) List(_ context.Context, actor string, _ Filter) (List, error) {
	f.calls++
	f.actor = actor
	return List{}, f.err
}
func (f *fakeStore) MarkRead(_ context.Context, actor string, _ ReadOptions) (ReadResult, error) {
	f.calls++
	f.actor = actor
	return ReadResult{}, f.err
}
func TestValidation(t *testing.T) {
	ctx := context.Background()
	for _, f := range []Filter{{0, 20, false}, {1, 0, false}, {1, 101, false}, {2147483647, 100, false}} {
		store := &fakeStore{}
		if _, err := New(store).List(ctx, "actor", f); !errors.Is(err, ErrValidation) || store.calls != 0 {
			t.Fatalf("filter %+v: %v", f, err)
		}
	}
	for _, o := range []ReadOptions{{}, {All: true, IDs: []string{"00000000-0000-0000-0000-000000000000"}}, {IDs: []string{"bad"}}, {IDs: make([]string, 101)}, {IDs: []string{"zzzzzzzz-0000-0000-0000-000000000000"}}} {
		store := &fakeStore{}
		if _, err := New(store).MarkRead(ctx, "actor", o); !errors.Is(err, ErrValidation) || store.calls != 0 {
			t.Fatalf("options %+v: %v", o, err)
		}
	}
	store := &fakeStore{}
	s := New(store)
	if _, err := s.List(ctx, "actor", Filter{1, 100, true}); err != nil {
		t.Fatal(err)
	}
	for _, o := range []ReadOptions{{All: true}, {IDs: []string{"AB123456-0000-0000-0000-000000000000"}}} {
		if _, err := s.MarkRead(ctx, "actor", o); err != nil {
			t.Fatal(err)
		}
	}
	if store.calls != 3 || store.actor != "actor" {
		t.Fatal(store)
	}
	sentinel := errors.New("database failure")
	store.err = sentinel
	if _, err := s.List(ctx, "actor", Filter{1, 20, false}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := s.MarkRead(ctx, "actor", ReadOptions{All: true}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
