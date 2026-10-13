-- name: GetMigrationState :one
SELECT version, dirty FROM schema_migrations LIMIT 1;
