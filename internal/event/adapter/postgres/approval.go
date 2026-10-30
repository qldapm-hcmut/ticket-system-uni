package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

func (s *Store) DoApproval(ctx context.Context, fn func(application.ApprovalRepository) error) error {
	return s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { return fn(&approvalRepo{q: s.q.WithTx(tx)}) })
}

type approvalRepo struct{ q *db.Queries }

func (r *approvalRepo) Get(ctx context.Context, raw string, lock bool) (domain.Event, error) {
	u, err := id(raw)
	if err != nil {
		return domain.Event{}, err
	}
	if lock {
		return event(r.q.GetEventForUpdate(ctx, u))
	}
	return event(r.q.GetEvent(ctx, u))
}
func (r *approvalRepo) IsOrganizer(ctx context.Context, user, unit string) (bool, error) {
	u, err := id(user)
	if err != nil {
		return false, err
	}
	v, err := id(unit)
	if err != nil {
		return false, err
	}
	return r.q.IsUnitOrganizer(ctx, db.IsUnitOrganizerParams{UserID: u, UnitID: v})
}
func (r *approvalRepo) Submit(ctx context.Context, raw string, v int32) (domain.Event, error) {
	u, err := id(raw)
	if err != nil {
		return domain.Event{}, err
	}
	return event(r.q.SubmitEvent(ctx, db.SubmitEventParams{ID: u, Version: v}))
}
func (r *approvalRepo) Approve(ctx context.Context, raw string, v int32, admin string) (domain.Event, error) {
	u, err := id(raw)
	if err != nil {
		return domain.Event{}, err
	}
	a, err := id(admin)
	if err != nil {
		return domain.Event{}, err
	}
	return event(r.q.ApproveEvent(ctx, db.ApproveEventParams{ID: u, ApprovedBy: a, Version: v}))
}
func (r *approvalRepo) Reject(ctx context.Context, raw string, v int32, reason string) (domain.Event, error) {
	u, err := id(raw)
	if err != nil {
		return domain.Event{}, err
	}
	return event(r.q.RejectEvent(ctx, db.RejectEventParams{ID: u, RejectReason: text(&reason), Version: v}))
}
func (r *approvalRepo) Pending(ctx context.Context, page, size int32) ([]domain.Event, int64, error) {
	rows, err := r.q.ListPendingEvents(ctx, db.ListPendingEventsParams{Limit: size, Offset: (page - 1) * size})
	if err != nil {
		return nil, 0, err
	}
	count, err := r.q.CountPendingEvents(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		e, err := event(row, nil)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, count, nil
}
