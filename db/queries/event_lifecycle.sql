-- name: LockExpiredWaitlistEvents :many
SELECT id FROM events
WHERE start_at <= now() AND (waitlist_count > 0 OR EXISTS (
 SELECT 1 FROM registrations r WHERE r.event_id=events.id AND r.status='WAITLISTED'))
ORDER BY start_at,id LIMIT sqlc.arg(batch_size)
FOR UPDATE SKIP LOCKED;

-- name: ExpireEventWaitlist :execrows
UPDATE registrations SET status='EXPIRED',updated_at=now()
WHERE event_id=$1 AND status='WAITLISTED';

-- name: ClearEventWaitlistCount :exec
UPDATE events SET waitlist_count=0,updated_at=now() WHERE id=$1;

-- name: LockCompletedEvents :many
SELECT id FROM events WHERE status='PUBLISHED' AND end_at < now()
ORDER BY end_at,id LIMIT sqlc.arg(batch_size)
FOR UPDATE SKIP LOCKED;

-- name: CompletePublishedEvent :execrows
UPDATE events SET status='COMPLETED',version=version+1,updated_at=now()
WHERE id=$1 AND status='PUBLISHED' AND end_at < now();
