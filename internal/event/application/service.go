package application

import (
	"context"

	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
)

type Actor struct {
	ID          string
	IsAdmin     bool
	IsOrganizer bool
}
type Repository interface {
	IsOrganizer(context.Context, string, string) (bool, error)
	Get(context.Context, string, bool) (domain.Event, error)
	Create(context.Context, string, string, domain.Fields) (domain.Event, error)
	Update(context.Context, domain.Event) (domain.Event, error)
}
type UnitOfWork interface {
	Do(context.Context, func(Repository) error) error
}
type Service struct {
	uow   UnitOfWork
	clock kernel.Clock
}

func New(uow UnitOfWork, clock kernel.Clock) *Service { return &Service{uow, clock} }
func (s *Service) Create(ctx context.Context, actor Actor, unit string, f domain.Fields) (domain.Event, error) {
	if !actor.IsOrganizer {
		return domain.Event{}, domain.ErrForbidden
	}
	if err := f.Validate(s.clock.Now()); err != nil {
		return domain.Event{}, err
	}
	var result domain.Event
	err := s.uow.Do(ctx, func(r Repository) error {
		allowed, err := r.IsOrganizer(ctx, actor.ID, unit)
		if err != nil {
			return err
		}
		if !allowed {
			return domain.ErrNotFound
		}
		result, err = r.Create(ctx, unit, actor.ID, f)
		return err
	})
	if err != nil {
		return domain.Event{}, err
	}
	return result, nil
}
func (s *Service) Get(ctx context.Context, actor Actor, id string) (domain.Event, error) {
	var result domain.Event
	err := s.uow.Do(ctx, func(r Repository) error {
		e, err := r.Get(ctx, id, false)
		if err != nil {
			return err
		}
		if !actor.IsAdmin {
			allowed, err := r.IsOrganizer(ctx, actor.ID, e.UnitID)
			if err != nil {
				return err
			}
			if !allowed {
				return domain.ErrNotFound
			}
		}
		result = e
		return nil
	})
	return result, err
}

// Patch runs parsing/merging after fetching the current version under a row lock.
func (s *Service) Patch(ctx context.Context, actor Actor, id string, version int32, merge func(domain.Fields) (domain.Fields, error)) (domain.Event, error) {
	if !actor.IsOrganizer {
		return domain.Event{}, domain.ErrForbidden
	}
	var result domain.Event
	err := s.uow.Do(ctx, func(r Repository) error {
		e, err := r.Get(ctx, id, true)
		if err != nil {
			return err
		}
		allowed, err := r.IsOrganizer(ctx, actor.ID, e.UnitID)
		if err != nil {
			return err
		}
		if !allowed {
			return domain.ErrNotFound
		}
		if e.Version != version {
			return domain.ErrVersion
		}
		if e.Status != "DRAFT" && e.Status != "REJECTED" {
			return domain.ErrState
		}
		e.Fields, err = merge(e.Fields)
		if err != nil {
			return err
		}
		if err = e.Fields.Validate(s.clock.Now()); err != nil {
			return err
		}
		result, err = r.Update(ctx, e)
		return err
	})
	if err != nil {
		return domain.Event{}, err
	}
	return result, nil
}
