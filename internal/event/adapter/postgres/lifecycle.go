package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const lifecycleBatchSize = 100

// ExpireWaitlists takes event locks before registration locks, matching the
// interactive registration/cancellation paths. Each batch commits atomically.
func (s *Store) ExpireWaitlists(ctx context.Context) (int64, error) {
	return s.runLifecycle(ctx, false)
}
func (s *Store) CompleteEvents(ctx context.Context) (int64, error) {
	return s.runLifecycle(ctx, true)
}
func (s *Store) runLifecycle(ctx context.Context, complete bool) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		var processed int
		var changed int64
		err := s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			q := s.q.WithTx(tx)
			var ids []pgtype.UUID
			var err error
			if complete {
				ids, err = q.LockCompletedEvents(ctx, lifecycleBatchSize)
			} else {
				ids, err = q.LockExpiredWaitlistEvents(ctx, lifecycleBatchSize)
			}
			if err != nil {
				return err
			}
			processed = len(ids)
			for _, id := range ids {
				n, err := q.ExpireEventWaitlist(ctx, id)
				if err != nil {
					return err
				}
				if err := q.ClearEventWaitlistCount(ctx, id); err != nil {
					return err
				}
				if complete {
					n, err = q.CompletePublishedEvent(ctx, id)
					if err != nil {
						return err
					}
				}
				changed += n
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		total += changed
		if processed < lifecycleBatchSize {
			return total, nil
		}
	}
}
