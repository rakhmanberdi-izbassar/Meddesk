-- name: AuthFindUserByEmail :one
SELECT u.id::text AS id, u.email, u.display_name, r.code AS role,
       COALESCE(u.department_id::text,'')::text AS department_id, COALESCE(d.name,'')::text AS department_name,
       u.account_scope, u.is_active, u.password_hash
FROM users u JOIN roles r ON r.id = u.role_id LEFT JOIN departments d ON d.id = u.department_id
WHERE lower(u.email) = sqlc.arg(email);

-- name: AuthFindSession :one
SELECT u.id::text AS id, u.email, u.display_name, r.code AS role,
       COALESCE(u.department_id::text,'')::text AS department_id, COALESCE(d.name,'')::text AS department_name,
       u.account_scope, u.is_active, s.csrf_token_hash, s.expires_at
FROM sessions s JOIN users u ON u.id = s.user_id JOIN roles r ON r.id = u.role_id
LEFT JOIN departments d ON d.id = u.department_id
WHERE s.token_hash = sqlc.arg(token_hash) AND s.expires_at > now() AND u.is_active;

-- name: AuthLockLoginUser :one
SELECT u.id::text AS id, u.email, u.display_name, r.code AS role,
       COALESCE(u.department_id::text,'')::text AS department_id, COALESCE(d.name,'')::text AS department_name,
       u.account_scope, u.is_active, u.password_hash
FROM users u JOIN roles r ON r.id = u.role_id LEFT JOIN departments d ON d.id = u.department_id
WHERE u.id = sqlc.arg(user_id)::uuid FOR UPDATE OF u;

-- name: AuthCreateSession :exec
INSERT INTO sessions (token_hash, user_id, csrf_token_hash, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(user_id)::uuid, sqlc.arg(csrf_token_hash), sqlc.arg(expires_at));

-- name: AuthFindSessionOwner :one
SELECT user_id FROM sessions WHERE token_hash = sqlc.arg(token_hash);

-- name: AuthDeleteSession :one
DELETE FROM sessions WHERE token_hash = sqlc.arg(token_hash) RETURNING user_id::text;

-- name: AuthDeleteExpiredUserSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id)::uuid AND expires_at <= now();

-- name: AuthRecordEvent :exec
WITH clinic_event AS (
 INSERT INTO audit_logs(actor_id,action,entity_type,entity_id)
 SELECT id,sqlc.arg(action),'session',id FROM users WHERE id=sqlc.arg(user_id)::uuid AND account_scope='CLINIC'
)
INSERT INTO school_account_events(actor_id,user_id,action)
SELECT id,id,sqlc.arg(action) FROM users WHERE id=sqlc.arg(user_id)::uuid AND account_scope='SCHOOL';
