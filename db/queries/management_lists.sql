-- name: GetManagementScope :one
SELECT e.id FROM events e JOIN users u ON u.id=sqlc.arg(actor_id) AND u.is_active
WHERE e.id=sqlc.arg(event_id) AND (u.is_admin OR EXISTS (
 SELECT 1 FROM unit_memberships m JOIN units un ON un.id=m.unit_id AND un.is_active
 WHERE m.user_id=u.id AND m.unit_id=e.unit_id));

-- name: ListEventRegistrationViews :many
WITH ranked AS (
 SELECT r.*, CASE WHEN r.status='WAITLISTED' AND now()<e.start_at
 THEN row_number() OVER (PARTITION BY r.status ORDER BY r.queue_seq,r.id) END AS position,
 CASE WHEN r.status='WAITLISTED' AND now()>=e.start_at THEN 'EXPIRED' ELSE r.status::text END AS effective_status
 FROM registrations r JOIN events e ON e.id=r.event_id WHERE r.event_id=sqlc.arg(event_id)
)
SELECT jsonb_build_object('id',r.id,'student',jsonb_build_object('id',u.id,'fullName',u.full_name,'studentCode',u.student_code,'email',u.email),
 'status',r.effective_status,'waitlistPosition',r.position,'createdAt',r.created_at,'promotedAt',r.promoted_at,
 'cancelledAt',r.cancelled_at,'cancellationExceptionUntil',CASE WHEN r.cancellation_exception_until>now() AND now()<e.start_at THEN r.cancellation_exception_until END,
 'checkedInAt',r.checked_in_at) AS item
FROM ranked r JOIN users u ON u.id=r.student_id JOIN events e ON e.id=r.event_id
WHERE (sqlc.arg(status_filter)::text='' OR r.effective_status=sqlc.arg(status_filter))
 AND (sqlc.arg(search)::text='' OR strpos(lower(u.full_name),lower(sqlc.arg(search)))>0 OR strpos(lower(coalesce(u.student_code,'')),lower(sqlc.arg(search)))>0)
ORDER BY r.queue_seq,r.id
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: ListEventCheckinViews :many
SELECT jsonb_build_object('registrationId',r.id,'method',r.checkin_method,'checkedInAt',r.checked_in_at,'note',r.checkin_note,
 'student',jsonb_build_object('fullName',u.full_name,'studentCode',u.student_code),
 'checkedInBy',jsonb_build_object('id',actor.id,'fullName',actor.full_name)) AS item
FROM registrations r JOIN users u ON u.id=r.student_id JOIN users actor ON actor.id=r.checked_in_by
WHERE r.event_id=sqlc.arg(event_id) AND r.checked_in_at IS NOT NULL
 AND (sqlc.arg(method_filter)::text='' OR r.checkin_method::text=sqlc.arg(method_filter))
ORDER BY r.checked_in_at DESC,r.id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: ListEventApprovalViews :many
SELECT jsonb_build_object('id',l.id,'action',l.action,'actor',jsonb_build_object('id',u.id,'fullName',u.full_name),
 'comment',l.metadata->>'reason','createdAt',l.created_at) AS item
FROM activity_logs l LEFT JOIN users u ON u.id=l.actor_id
WHERE l.event_id=$1 AND l.action IN ('SUBMIT','APPROVE','REJECT')
ORDER BY l.created_at,l.id;

-- name: CountEventRegistrationViews :one
SELECT count(*) FROM registrations r JOIN users u ON u.id=r.student_id JOIN events e ON e.id=r.event_id
WHERE r.event_id=sqlc.arg(event_id)
 AND (sqlc.arg(status_filter)::text='' OR (CASE WHEN r.status='WAITLISTED' AND now()>=e.start_at THEN 'EXPIRED' ELSE r.status::text END)=sqlc.arg(status_filter))
 AND (sqlc.arg(search)::text='' OR strpos(lower(u.full_name),lower(sqlc.arg(search)))>0 OR strpos(lower(coalesce(u.student_code,'')),lower(sqlc.arg(search)))>0);

-- name: CountEventCheckinViews :one
SELECT count(*) FROM registrations r WHERE r.event_id=sqlc.arg(event_id) AND r.checked_in_at IS NOT NULL
 AND (sqlc.arg(method_filter)::text='' OR r.checkin_method::text=sqlc.arg(method_filter));

-- name: AuditApprovalAction :exec
INSERT INTO activity_logs(actor_id,event_id,entity_type,entity_id,action,metadata)
VALUES(sqlc.arg(actor_id),sqlc.arg(event_id),'EVENT',sqlc.arg(event_id),sqlc.arg(action),sqlc.arg(metadata)::jsonb);
