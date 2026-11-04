-- name: ListMyNotifications :many
SELECT id,event_id,type,title,body,read_at,created_at
FROM notifications
WHERE user_id=sqlc.arg(user_id) AND (NOT sqlc.arg(unread_only)::boolean OR read_at IS NULL)
ORDER BY created_at DESC,id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountMyNotifications :one
SELECT count(*) FILTER (WHERE NOT sqlc.arg(unread_only)::boolean OR read_at IS NULL) AS total_items,
 count(*) FILTER (WHERE read_at IS NULL) AS unread_count
FROM notifications WHERE user_id=sqlc.arg(user_id);

-- name: MarkMyNotificationsRead :execrows
UPDATE notifications SET read_at=now()
WHERE user_id=sqlc.arg(user_id) AND read_at IS NULL
 AND (sqlc.arg(read_all)::boolean OR id=ANY(sqlc.arg(ids)::uuid[]));
