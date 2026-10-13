-- name: GetApplicationSettings :one
SELECT clinic_name, default_ticket_priority, reporting_timezone, version, updated_at FROM application_settings WHERE singleton = true;

-- name: GetApplicationSettingsForUpdate :one
SELECT clinic_name, default_ticket_priority, reporting_timezone, version, updated_at FROM application_settings WHERE singleton = true FOR UPDATE;

-- name: UpdateApplicationSettings :one
UPDATE application_settings SET clinic_name = $1, default_ticket_priority = $2, reporting_timezone = $3, version = version + 1
WHERE singleton = true AND version = $4
RETURNING clinic_name, default_ticket_priority, reporting_timezone, version, updated_at;

-- name: ReportingActor :one
SELECT u.id, r.code AS role, u.is_active FROM users u JOIN roles r ON r.id = u.role_id WHERE u.id = $1;
