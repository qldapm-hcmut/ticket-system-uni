-- name: ListManagedEvents :many
SELECT e.* FROM events e
WHERE (sqlc.arg(is_admin)::boolean OR EXISTS(SELECT 1 FROM unit_memberships m WHERE m.unit_id=e.unit_id AND m.user_id=sqlc.arg(actor_id)))
AND (sqlc.arg(unit_filter)::text='' OR e.unit_id::text=sqlc.arg(unit_filter))
AND (sqlc.arg(status_filter)::text='' OR e.status::text=ANY(string_to_array(sqlc.arg(status_filter),',')))
AND (sqlc.arg(search)::text='' OR strpos(lower(e.title),lower(sqlc.arg(search)::text))>0)
ORDER BY e.start_at,e.id LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountManagedEvents :one
SELECT count(*) FROM events e
WHERE (sqlc.arg(is_admin)::boolean OR EXISTS(SELECT 1 FROM unit_memberships m WHERE m.unit_id=e.unit_id AND m.user_id=sqlc.arg(actor_id)))
AND (sqlc.arg(unit_filter)::text='' OR e.unit_id::text=sqlc.arg(unit_filter))
AND (sqlc.arg(status_filter)::text='' OR e.status::text=ANY(string_to_array(sqlc.arg(status_filter),',')))
AND (sqlc.arg(search)::text='' OR strpos(lower(e.title),lower(sqlc.arg(search)::text))>0);

-- name: GetEventUnit :one
SELECT id,code,name FROM units WHERE id=$1;

-- name: GetEventCreator :one
SELECT id,full_name FROM users WHERE id=$1;
