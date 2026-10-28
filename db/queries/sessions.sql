-- name: CreateAuthSession :one
INSERT INTO auth_sessions (user_id, refresh_token_hash, expires_at)
VALUES ($1, $2, $3) RETURNING *;

-- name: ConsumeAuthSession :one
UPDATE auth_sessions SET revoked_at = sqlc.arg(now_at)
WHERE refresh_token_hash = sqlc.arg(token_hash)
  AND revoked_at IS NULL AND expires_at > sqlc.arg(now_at)
  AND EXISTS (SELECT 1 FROM users WHERE users.id = auth_sessions.user_id AND is_active = true)
RETURNING *;

-- name: RevokeAuthSession :exec
UPDATE auth_sessions SET revoked_at = sqlc.arg(now_at)
WHERE refresh_token_hash = sqlc.arg(token_hash)
  AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL;
