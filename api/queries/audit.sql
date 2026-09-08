-- name: InsertAuditLog :exec
INSERT INTO audit_logs
    (actor_user_id, action, entity_type, entity_id, before, after, ip, user_agent, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::text   IS NULL OR entity_id   = sqlc.narg('entity_id')::text)
  AND (sqlc.narg('actor')::bigint     IS NULL OR actor_user_id = sqlc.narg('actor')::bigint)
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');
