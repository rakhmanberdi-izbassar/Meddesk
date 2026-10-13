-- name: ListManagedUsers :many
SELECT u.id, u.email, u.display_name, r.code AS role, u.department_id,
       d.name AS department_name, u.is_active
FROM users u JOIN roles r ON r.id = u.role_id JOIN departments d ON d.id = u.department_id
WHERE (sqlc.arg(search)::text = '' OR strpos(lower(u.email || ' ' || u.display_name), lower(sqlc.arg(search))) > 0)
  AND (sqlc.arg(role)::text = '' OR r.code = sqlc.arg(role))
  AND (sqlc.narg(active)::boolean IS NULL OR u.is_active = sqlc.narg(active))
ORDER BY u.created_at DESC, u.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountManagedUsers :one
SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id
WHERE (sqlc.arg(search)::text = '' OR strpos(lower(u.email || ' ' || u.display_name), lower(sqlc.arg(search))) > 0)
  AND (sqlc.arg(role)::text = '' OR r.code = sqlc.arg(role))
  AND (sqlc.narg(active)::boolean IS NULL OR u.is_active = sqlc.narg(active));

-- name: GetManagedUser :one
SELECT u.id, u.email, u.display_name, r.code AS role, u.department_id,
       d.name AS department_name, u.is_active
FROM users u JOIN roles r ON r.id = u.role_id JOIN departments d ON d.id = u.department_id
WHERE u.id = $1;

-- name: GetManagedUserForUpdate :one
SELECT u.id, u.email, u.display_name, r.code AS role, u.department_id,
       d.name AS department_name, u.is_active
FROM users u JOIN roles r ON r.id = u.role_id JOIN departments d ON d.id = u.department_id
WHERE u.id = $1 FOR UPDATE OF u;

-- name: CreateManagedUser :one
INSERT INTO users (email, display_name, password_hash, role_id, department_id)
VALUES (sqlc.arg(email), sqlc.arg(display_name), sqlc.arg(password_hash),
        (SELECT id FROM roles WHERE code = sqlc.arg(role)), sqlc.arg(department_id))
RETURNING id;

-- name: UpdateManagedUser :exec
UPDATE users SET email = sqlc.arg(email), display_name = sqlc.arg(display_name),
    role_id = (SELECT id FROM roles WHERE code = sqlc.arg(role)),
    department_id = sqlc.arg(department_id), is_active = sqlc.arg(is_active),
    password_hash = COALESCE(sqlc.narg(password_hash)::text, password_hash)
WHERE users.id = sqlc.arg(id);

-- name: UpdateOwnProfile :exec
UPDATE users SET display_name = $2 WHERE id = $1;

-- name: CountActiveAdministrators :one
SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id
WHERE u.is_active AND r.code = 'ADMIN';

-- name: LockUserManagement :exec
SELECT pg_advisory_xact_lock(628004271);

-- name: RevokeManagedUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: InsertManagementAudit :exec
INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, metadata)
VALUES ($1, $2, $3, $4, $5);
