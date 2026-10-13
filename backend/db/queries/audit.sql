-- name: ListAuditLogs :many
SELECT a.id, a.actor_id, u.display_name AS actor_name, a.action, a.entity_type, a.entity_id, a.metadata, a.created_at
FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
WHERE a.created_at >= sqlc.arg(start_at)::timestamptz AND a.created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(actor_id)::uuid IS NULL OR a.actor_id = sqlc.narg(actor_id))
 AND (sqlc.arg(action)::text = '' OR a.action = sqlc.arg(action))
 AND (sqlc.arg(entity_type)::text = '' OR a.entity_type = sqlc.arg(entity_type))
 AND (sqlc.narg(entity_id)::uuid IS NULL OR a.entity_id = sqlc.narg(entity_id))
ORDER BY a.created_at DESC, a.id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAuditLogs :one
SELECT count(*) FROM audit_logs a
WHERE a.created_at >= sqlc.arg(start_at)::timestamptz AND a.created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(actor_id)::uuid IS NULL OR a.actor_id = sqlc.narg(actor_id))
 AND (sqlc.arg(action)::text = '' OR a.action = sqlc.arg(action))
 AND (sqlc.arg(entity_type)::text = '' OR a.entity_type = sqlc.arg(entity_type))
 AND (sqlc.narg(entity_id)::uuid IS NULL OR a.entity_id = sqlc.narg(entity_id));

-- name: AuditFilterActions :many
SELECT DISTINCT action FROM audit_logs ORDER BY action;

-- name: AuditFilterEntityTypes :many
SELECT DISTINCT entity_type FROM audit_logs ORDER BY entity_type;

-- name: AuditFilterActors :many
SELECT DISTINCT u.id, u.display_name FROM users u JOIN audit_logs a ON a.actor_id = u.id ORDER BY u.display_name, u.id;
