-- name: NotifyEventChanged :execrows
INSERT INTO notifications(user_id,event_id,type,title,body)
SELECT student_id,event_id,'EVENT_CHANGED','Event updated','The time or location of your event has changed.'
FROM registrations r WHERE r.event_id=$1 AND status IN ('REGISTERED','WAITLISTED');

-- name: UpdateCancellationExceptions :execrows
UPDATE registrations SET cancellation_exception_until=sqlc.arg(new_start)::timestamptz,updated_at=now()
WHERE event_id=sqlc.arg(event_id) AND status='REGISTERED'
AND (sqlc.arg(grant_exception)::boolean OR cancellation_exception_until>now());

-- name: AuditEventChange :exec
INSERT INTO activity_logs(actor_id,event_id,entity_type,entity_id,action,metadata)
VALUES(sqlc.arg(actor_id),sqlc.arg(event_id),'EVENT',sqlc.arg(event_id),'UPDATE_EVENT',sqlc.arg(metadata)::jsonb);
