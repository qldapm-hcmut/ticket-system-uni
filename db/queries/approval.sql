-- name: SubmitEvent :one
UPDATE events SET status='PENDING_APPROVAL', reject_reason=NULL, version=version+1, updated_at=now()
WHERE id=$1 AND version=$2 AND status IN ('DRAFT','REJECTED') RETURNING *;

-- name: ApproveEvent :one
UPDATE events SET status='PUBLISHED', approved_by=$2, approved_at=now(), published_at=now(), version=version+1, updated_at=now()
WHERE id=$1 AND version=$3 AND status='PENDING_APPROVAL' AND created_by<>$2 RETURNING *;

-- name: RejectEvent :one
UPDATE events SET status='REJECTED', reject_reason=$2, approved_by=NULL, approved_at=NULL, version=version+1, updated_at=now()
WHERE id=$1 AND version=$3 AND status='PENDING_APPROVAL' RETURNING *;

-- name: ListPendingEvents :many
SELECT * FROM events WHERE status='PENDING_APPROVAL' ORDER BY updated_at,id LIMIT $1 OFFSET $2;
-- name: CountPendingEvents :one
SELECT count(*) FROM events WHERE status='PENDING_APPROVAL';
