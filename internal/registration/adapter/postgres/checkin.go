package postgres

import (
	"context"
	"errors"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/registration/application"
)

func (s *Store) CheckIn(ctx context.Context, actor, event string, in a.CheckinInput) (a.CheckinResult, error) {
	var out a.CheckinResult
	staff, err := uuid(actor)
	if err != nil {
		return out, err
	}
	eid, err := uuid(event)
	if err != nil {
		return out, err
	}
	policy := a.CheckinPolicy{Clock: kernel.SystemClock{}}
	if err := policy.ValidateInput(in); err != nil {
		return out, err
	}
	err = s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		e, err := q.GetEvent(ctx, eid)
		if errors.Is(err, pgx.ErrNoRows) {
			return a.ErrNotFound
		}
		if err != nil {
			return err
		}
		allowed, err := q.IsUnitOrganizer(ctx, db.IsUnitOrganizerParams{UserID: staff, UnitID: e.UnitID})
		if err != nil {
			return err
		}
		if !allowed {
			return a.ErrNotFound
		}
		var reg db.Registration
		if in.Method == "CODE" {
			reg, err = q.GetRegistrationByCheckinCode(ctx, db.GetRegistrationByCheckinCodeParams{EventID: eid, Code: in.Code})
		} else {
			rid, parseErr := uuid(in.RegistrationID)
			if parseErr != nil {
				return parseErr
			}
			reg, err = q.GetRegistration(ctx, rid)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return a.ErrNotFound
		}
		if err != nil {
			return err
		}
		if reg.EventID != eid {
			return a.ErrNotFound
		}
		if err := policy.Authorize(actor, allowed, a.CheckinState{StudentID: reg.StudentID.String(), EventStatus: string(e.Status), RegistrationStatus: string(reg.Status), StartAt: e.StartAt.Time, EndAt: e.EndAt.Time}, false); err != nil {
			return err
		}
		if reg.CheckedInAt.Valid {
			return &a.AlreadyCheckedIn{At: reg.CheckedInAt.Time, By: reg.CheckedInBy.String()}
		}
		note := pgtype.Text{}
		if in.Method == "MANUAL" {
			note = pgtype.Text{String: in.Note, Valid: true}
		}
		updated, err := q.CheckInRegistration(ctx, db.CheckInRegistrationParams{RegistrationID: reg.ID, EventID: eid, StaffID: staff, Method: db.CheckinMethod(in.Method), Note: note})
		if errors.Is(err, pgx.ErrNoRows) {
			latest, readErr := q.GetRegistration(ctx, reg.ID)
			if readErr != nil {
				return readErr
			}
			if latest.CheckedInAt.Valid {
				return &a.AlreadyCheckedIn{At: latest.CheckedInAt.Time, By: latest.CheckedInBy.String()}
			}
			return a.ErrCheckinEligibility
		}
		if err != nil {
			return err
		}
		if in.Method == "MANUAL" {
			if err = q.AuditManualCheckin(ctx, reg.ID); err != nil {
				return err
			}
		}
		var staffName, studentName string
		var code pgtype.Text
		if err = tx.QueryRow(ctx, "SELECT full_name FROM users WHERE id=$1", staff).Scan(&staffName); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT full_name,student_code FROM users WHERE id=$1", reg.StudentID).Scan(&studentName, &code); err != nil {
			return err
		}
		out = a.CheckinResult{RegistrationID: reg.ID.String(), Method: in.Method, CheckedInAt: updated.CheckedInAt.Time, CheckedInBy: a.CheckinPerson{ID: actor, FullName: staffName}, Student: a.CheckinPerson{FullName: studentName, StudentCode: code.String}}
		return nil
	})
	return out, translate(err)
}
func (s *Store) UndoCheckIn(ctx context.Context, actor, registration string) error {
	staff, err := uuid(actor)
	if err != nil {
		return err
	}
	rid, err := uuid(registration)
	if err != nil {
		return err
	}
	return s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		reg, err := q.GetRegistration(ctx, rid)
		if errors.Is(err, pgx.ErrNoRows) {
			return a.ErrNotFound
		}
		if err != nil {
			return err
		}
		event, err := q.GetEvent(ctx, reg.EventID)
		if err != nil {
			return err
		}
		allowed, err := q.IsUnitOrganizer(ctx, db.IsUnitOrganizerParams{UserID: staff, UnitID: event.UnitID})
		if err != nil {
			return err
		}
		if !allowed {
			return a.ErrNotFound
		}
		if err := (a.CheckinPolicy{}).Authorize(actor, allowed, a.CheckinState{StudentID: reg.StudentID.String()}, true); err != nil {
			return err
		}
		_, err = q.UndoRegistrationCheckin(ctx, db.UndoRegistrationCheckinParams{RegistrationID: rid, ActorID: staff})
		if errors.Is(err, pgx.ErrNoRows) {
			return a.ErrNotFound
		}
		return err
	})
}
