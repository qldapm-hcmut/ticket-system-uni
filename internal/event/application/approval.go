package application

import (
	"context"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

type ApprovalRepository interface {
	Get(context.Context, string, bool) (domain.Event, error)
	IsOrganizer(context.Context, string, string) (bool, error)
	Submit(context.Context, string, int32) (domain.Event, error)
	Approve(context.Context, string, int32, string) (domain.Event, error)
	Reject(context.Context, string, int32, string) (domain.Event, error)
	Pending(context.Context, int32, int32) ([]domain.Event, int64, error)
}
type ApprovalUnitOfWork interface {
	DoApproval(context.Context, func(ApprovalRepository) error) error
}
type ApprovalService struct{ uow ApprovalUnitOfWork }

func NewApprovalService(u ApprovalUnitOfWork) *ApprovalService { return &ApprovalService{u} }
func (s *ApprovalService) Submit(ctx context.Context, a Actor, id string, version int32) (domain.Event, error) {
	var out domain.Event
	err := s.uow.DoApproval(ctx, func(r ApprovalRepository) error {
		e, err := r.Get(ctx, id, true)
		if err != nil {
			return err
		}
		ok, err := r.IsOrganizer(ctx, a.ID, e.UnitID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrNotFound
		}
		if version != 0 && e.Version != version {
			return domain.ErrVersion
		}
		if e.Status != "DRAFT" && e.Status != "REJECTED" {
			return domain.ErrState
		}
		out, err = r.Submit(ctx, id, e.Version)
		return err
	})
	return out, err
}
func (s *ApprovalService) Approve(ctx context.Context, a Actor, id string, version int32) (domain.Event, error) {
	if !a.IsAdmin {
		return domain.Event{}, domain.ErrForbidden
	}
	var out domain.Event
	err := s.uow.DoApproval(ctx, func(r ApprovalRepository) error {
		e, err := r.Get(ctx, id, true)
		if err != nil {
			return err
		}
		if e.CreatedBy == a.ID {
			return domain.ErrForbidden
		}
		if version != 0 && e.Version != version {
			return domain.ErrVersion
		}
		if e.Status != "PENDING_APPROVAL" {
			return domain.ErrState
		}
		out, err = r.Approve(ctx, id, e.Version, a.ID)
		return err
	})
	return out, err
}
func (s *ApprovalService) Reject(ctx context.Context, a Actor, id string, version int32, reason string) (domain.Event, error) {
	if !a.IsAdmin || len(reason) == 0 || len(reason) > 500 {
		return domain.Event{}, domain.ErrValidation
	}
	var out domain.Event
	err := s.uow.DoApproval(ctx, func(r ApprovalRepository) error {
		e, err := r.Get(ctx, id, true)
		if err != nil {
			return err
		}
		if version != 0 && e.Version != version {
			return domain.ErrVersion
		}
		if e.Status != "PENDING_APPROVAL" {
			return domain.ErrState
		}
		out, err = r.Reject(ctx, id, e.Version, reason)
		return err
	})
	return out, err
}
