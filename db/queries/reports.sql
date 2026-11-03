-- name: GetReportEvent :one
SELECT e.id, e.capacity, e.start_at
FROM events e JOIN users u ON u.id=sqlc.arg(actor_id) AND u.is_active
WHERE e.id=sqlc.arg(event_id) AND (u.is_admin OR EXISTS (
 SELECT 1 FROM unit_memberships m JOIN units un ON un.id=m.unit_id AND un.is_active
 WHERE m.user_id=u.id AND m.unit_id=e.unit_id));

-- name: GetReportSummary :one
SELECT count(*) FILTER (WHERE status='REGISTERED') AS registered,
 count(*) FILTER (WHERE status='WAITLISTED') AS waitlisted,
 count(*) FILTER (WHERE status IN ('CANCELLED','CANCELLED_BY_EVENT')) AS cancelled,
 count(*) FILTER (WHERE promoted_at IS NOT NULL) AS promoted,
 count(*) FILTER (WHERE checked_in_at IS NOT NULL) AS checked_in,
 count(*) FILTER (WHERE checkin_method='QR') AS qr,
 count(*) FILTER (WHERE checkin_method='CODE') AS code,
 count(*) FILTER (WHERE checkin_method='MANUAL') AS manual
FROM registrations WHERE event_id=$1;

-- name: ListReportRegistrations :many
SELECT u.student_code, u.full_name, u.email, r.status, r.checked_in_at, r.checkin_method
FROM registrations r JOIN users u ON u.id=r.student_id
WHERE r.event_id=sqlc.arg(event_id) AND (NOT sqlc.arg(attendance_only)::boolean OR r.checked_in_at IS NOT NULL)
ORDER BY r.created_at, r.id;

-- name: AuditReportExport :exec
INSERT INTO activity_logs(actor_id,event_id,entity_type,entity_id,action,metadata)
VALUES ($1,$2,'EVENT',$2,'EXPORT_DATA',sqlc.arg(metadata)::jsonb);
