package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/event/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

func (s *Store) ManagementList(ctx context.Context, actor, eventID, kind string, f a.ManagementFilter) (out a.ManagementPage, err error) {
	aid, err := id(actor)
	if err != nil {
		return out, domain.ErrNotFound
	}
	eid, err := id(eventID)
	if err != nil {
		return out, domain.ErrNotFound
	}
	err = s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY"); err != nil {
			return err
		}
		q := s.q.WithTx(tx)
		if _, err := q.GetManagementScope(ctx, db.GetManagementScopeParams{ActorID: aid, EventID: eid}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		var rows [][]byte
		var err error
		switch kind {
		case "registrations":
			out.Total, err = q.CountEventRegistrationViews(ctx, db.CountEventRegistrationViewsParams{EventID: eid, StatusFilter: f.Status, Search: f.Search})
			if err != nil {
				return err
			}
			rows, err = q.ListEventRegistrationViews(ctx, db.ListEventRegistrationViewsParams{EventID: eid, StatusFilter: f.Status, Search: f.Search, PageSize: f.PageSize, PageOffset: (f.Page - 1) * f.PageSize})
		case "check-ins":
			out.Total, err = q.CountEventCheckinViews(ctx, db.CountEventCheckinViewsParams{EventID: eid, MethodFilter: f.Method})
			if err != nil {
				return err
			}
			rows, err = q.ListEventCheckinViews(ctx, db.ListEventCheckinViewsParams{EventID: eid, MethodFilter: f.Method, PageSize: f.PageSize, PageOffset: (f.Page - 1) * f.PageSize})
		case "approval-logs":
			rows, err = q.ListEventApprovalViews(ctx, eid)
		default:
			return domain.ErrValidation
		}
		if err != nil {
			return err
		}
		out.Items = make([]map[string]any, 0, len(rows))
		for _, raw := range rows {
			var item map[string]any
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	if err != nil {
		return a.ManagementPage{}, err
	}
	return out, nil
}
