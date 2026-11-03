package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/notification/application"
	platform "github.com/qldapm-hcmut/ticket-system-uni/internal/platform/postgres"
)

type Store struct{ runner *platform.TransactionRunner }

func New(pool *pgxpool.Pool) *Store { return &Store{platform.NewTransactionRunner(pool)} }
func uuid(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(s)
	if err != nil || !id.Valid {
		return id, a.ErrValidation
	}
	return id, nil
}
func uuidString(id pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
func (s *Store) List(ctx context.Context, actor string, f a.Filter) (out a.List, err error) {
	id, err := uuid(actor)
	if err != nil {
		return out, err
	}
	err = s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY"); err != nil {
			return err
		}
		q := db.New(tx)
		counts, err := q.CountMyNotifications(ctx, db.CountMyNotificationsParams{UserID: id, UnreadOnly: f.Unread})
		if err != nil {
			return err
		}
		rows, err := q.ListMyNotifications(ctx, db.ListMyNotificationsParams{UserID: id, UnreadOnly: f.Unread, PageSize: f.PageSize, PageOffset: (f.Page - 1) * f.PageSize})
		if err != nil {
			return err
		}
		out = a.List{Items: make([]a.Notification, 0, len(rows)), TotalItems: counts.TotalItems, UnreadCount: counts.UnreadCount}
		for _, r := range rows {
			n := a.Notification{ID: uuidString(r.ID), Type: string(r.Type), Title: r.Title, Body: r.Body, CreatedAt: r.CreatedAt.Time}
			if r.EventID.Valid {
				v := uuidString(r.EventID)
				n.EventID = &v
			}
			if r.ReadAt.Valid {
				v := r.ReadAt.Time
				n.ReadAt = &v
			}
			out.Items = append(out.Items, n)
		}
		return nil
	})
	if err != nil {
		return a.List{}, err
	}
	return out, nil
}
func (s *Store) MarkRead(ctx context.Context, actor string, o a.ReadOptions) (out a.ReadResult, err error) {
	id, err := uuid(actor)
	if err != nil {
		return out, err
	}
	ids := make([]pgtype.UUID, 0, len(o.IDs))
	for _, raw := range o.IDs {
		v, err := uuid(raw)
		if err != nil {
			return out, err
		}
		ids = append(ids, v)
	}
	err = s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Serialize read acknowledgements for the same user, including mark-all.
		if _, err := tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", id); err != nil {
			return err
		}
		q := db.New(tx)
		updated, err := q.MarkMyNotificationsRead(ctx, db.MarkMyNotificationsReadParams{UserID: id, ReadAll: o.All, Ids: ids})
		if err != nil {
			return err
		}
		counts, err := q.CountMyNotifications(ctx, db.CountMyNotificationsParams{UserID: id})
		if err != nil {
			return err
		}
		out = a.ReadResult{UpdatedCount: updated, UnreadCount: counts.UnreadCount}
		return nil
	})
	if err != nil {
		return a.ReadResult{}, err
	}
	return out, nil
}
