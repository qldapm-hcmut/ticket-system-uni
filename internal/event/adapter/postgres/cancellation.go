package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

func (r *repository) Cancel(ctx context.Context, e domain.Event, actor, reason string) (domain.Event, int64, error) {
	eid, err := id(e.ID)
	if err != nil {
		return domain.Event{}, 0, err
	}
	aid, err := id(actor)
	if err != nil {
		return domain.Event{}, 0, err
	}
	updated, err := event(r.q.CancelPublishedEvent(ctx, db.CancelPublishedEventParams{EventID: eid, ExpectedVersion: e.Version, Reason: pgtype.Text{String: reason, Valid: true}}))
	if err != nil {
		return domain.Event{}, 0, err
	}
	affected, err := r.q.CancelEventRegistrations(ctx, db.CancelEventRegistrationsParams{EventID: eid, ActorID: aid, Reason: reason})
	if err != nil {
		return domain.Event{}, 0, err
	}
	metadata, err := json.Marshal(map[string]any{"reason": reason, "affectedCount": affected})
	if err != nil {
		return domain.Event{}, 0, err
	}
	if err := r.q.AuditEventCancellation(ctx, db.AuditEventCancellationParams{EventID: eid, ActorID: aid, Metadata: metadata}); err != nil {
		return domain.Event{}, 0, err
	}
	return updated, affected, nil
}
