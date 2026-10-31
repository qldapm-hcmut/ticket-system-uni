-- name: GetRegistrationByCheckinCode :one
SELECT r.* FROM registrations r
WHERE r.event_id=$1 AND r.checkin_code=upper(replace(sqlc.arg(code)::text,'-',''));

-- name: CheckInRegistration :one
UPDATE registrations r
SET checked_in_at=now(),checkin_method=sqlc.arg(method)::checkin_method,
    checked_in_by=sqlc.arg(staff_id),checkin_note=sqlc.narg(note)::text,updated_at=now()
WHERE r.id=sqlc.arg(registration_id) AND r.event_id=sqlc.arg(event_id)
  AND r.status='REGISTERED' AND r.checked_in_at IS NULL
RETURNING r.*;

-- name: UndoRegistrationCheckin :one
WITH previous AS (
 SELECT r.id,r.event_id,r.checked_in_at,r.checkin_method,r.checked_in_by,r.checkin_note
 FROM registrations r WHERE r.id=sqlc.arg(registration_id) AND r.checked_in_at IS NOT NULL
 FOR UPDATE
), undone AS (
 UPDATE registrations r SET checked_in_at=NULL,checkin_method=NULL,
 checked_in_by=NULL,checkin_note=NULL,updated_at=now()
 FROM previous p WHERE r.id=p.id RETURNING r.*
), audited AS (
 INSERT INTO activity_logs(actor_id,event_id,entity_type,entity_id,action,metadata)
 SELECT sqlc.arg(actor_id)::uuid,p.event_id,'REGISTRATION',p.id,'UNDO_CHECKIN',
 jsonb_build_object('checkedInAt',p.checked_in_at,'method',p.checkin_method,'checkedInBy',p.checked_in_by,'note',p.checkin_note)
 FROM previous p JOIN undone u ON u.id=p.id
)
SELECT * FROM undone;

-- name: AuditManualCheckin :exec
INSERT INTO activity_logs(actor_id,event_id,entity_type,entity_id,action,metadata)
SELECT r.checked_in_by,r.event_id,'REGISTRATION',r.id,'MANUAL_CHECKIN',
 jsonb_build_object('note',r.checkin_note,'checkedInAt',r.checked_in_at)
FROM registrations r WHERE r.id=$1 AND r.checkin_method='MANUAL' AND r.checked_in_at IS NOT NULL;
