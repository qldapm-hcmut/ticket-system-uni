-- name: CancelPublishedEvent :one
UPDATE events SET status='CANCELLED',cancelled_at=now(),cancel_reason=sqlc.arg(reason),
 registered_count=0,waitlist_count=0,version=version+1,updated_at=now()
WHERE id=sqlc.arg(event_id) AND status='PUBLISHED' AND version=sqlc.arg(expected_version)
RETURNING *;

-- name: CancelEventRegistrations :one
WITH affected AS (
 UPDATE registrations r SET status='CANCELLED_BY_EVENT',cancelled_at=now(),
 cancelled_by='ORGANIZER',cancelled_by_user=sqlc.arg(actor_id),
 cancel_reason='Event cancelled',cancellation_exception_until=NULL,updated_at=now()
 WHERE r.event_id=sqlc.arg(event_id) AND r.status IN ('REGISTERED','WAITLISTED')
 RETURNING r.student_id,r.event_id
), notified AS (
 INSERT INTO notifications(user_id,event_id,type,title,body)
 SELECT student_id,event_id,'EVENT_CANCELLED','Event cancelled',sqlc.arg(reason)::text FROM affected
 RETURNING id
)
SELECT count(*)::bigint AS affected_count FROM notified;

-- name: AuditEventCancellation :exec
INSERT INTO activity_logs(actor_id,event_id,entity_type,entity_id,action,metadata)
VALUES(sqlc.arg(actor_id),sqlc.arg(event_id),'EVENT',sqlc.arg(event_id),'CANCEL_EVENT',sqlc.arg(metadata)::jsonb);
