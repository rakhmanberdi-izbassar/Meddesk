-- name: ListManagedDepartments :many
SELECT id, name, is_active, created_at, updated_at FROM departments
WHERE (sqlc.arg(search)::text = '' OR strpos(lower(name), lower(sqlc.arg(search))) > 0)
  AND (sqlc.narg(active)::boolean IS NULL OR is_active = sqlc.narg(active))
ORDER BY name, id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountManagedDepartments :one
SELECT count(*) FROM departments
WHERE (sqlc.arg(search)::text = '' OR strpos(lower(name), lower(sqlc.arg(search))) > 0)
  AND (sqlc.narg(active)::boolean IS NULL OR is_active = sqlc.narg(active));

-- name: GetManagedDepartment :one
SELECT id, name, is_active, created_at, updated_at FROM departments WHERE id = $1;

-- name: GetManagedDepartmentForUpdate :one
SELECT id, name, is_active, created_at, updated_at FROM departments WHERE id = $1 FOR UPDATE;

-- name: CreateManagedDepartment :one
INSERT INTO departments (name) VALUES ($1) RETURNING id, name, is_active, created_at, updated_at;

-- name: UpdateManagedDepartment :one
UPDATE departments SET name = $2, is_active = $3 WHERE id = $1
RETURNING id, name, is_active, created_at, updated_at;

-- name: CountActiveDepartmentUsers :one
SELECT count(*) FROM users WHERE department_id = $1 AND is_active;
