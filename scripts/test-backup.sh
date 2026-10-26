#!/usr/bin/env bash
# Exercise backup and restoration only against synthetic disposable services.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
task_root=$PWD
task_temp=$(mktemp -d)
task_project="meddesk-backup-test-$(openssl rand -hex 6)"
task_password=$(openssl rand -hex 24)
export COMPOSE_PROJECT_NAME=$task_project
export COMPOSE_FILE="$task_temp/compose.yml"
cleanup() {
  docker compose down --volumes >/dev/null 2>&1 || true
  rm -rf -- "$task_temp"
}
trap cleanup EXIT
cat > "$COMPOSE_FILE" <<EOF
services:
  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_USER: backup_probe
      POSTGRES_DB: backup_probe
      POSTGRES_PASSWORD: $task_password
    volumes: [database:/var/lib/postgresql]
    healthcheck:
      test: [CMD-SHELL, 'pg_isready -U backup_probe -d backup_probe']
      interval: 1s
      timeout: 3s
      retries: 60
  backend:
    image: alpine:3.23
    environment:
      APP_ENV: development
    command: [sh, -c, 'exec sleep 600']
    volumes: [media:/var/lib/meddesk/media]
volumes:
  database:
  media:
EOF
docker compose up -d --wait --wait-timeout 90 >/dev/null
docker run --rm --network "${task_project}_default" -v "$task_root/backend/db/migrations:/migrations:ro" \
  migrate/migrate:v4.19.1 -path=/migrations \
  -database="postgres://backup_probe:$task_password@postgres:5432/backup_probe?sslmode=disable" up >/dev/null
docker compose exec -T postgres psql -X -U backup_probe -d backup_probe --set ON_ERROR_STOP=1 >/dev/null <<'SQL'
CREATE TABLE backup_probe_data (content text NOT NULL);
INSERT INTO backup_probe_data VALUES ('synthetic-backup-content');
SQL
docker compose exec -T backend sh -ec 'printf synthetic-media > /var/lib/meddesk/media/probe.txt'
bash scripts/backup-dev.sh "$task_temp/archive"
(cd "$task_temp/archive" && sha256sum --check checksums.sha256)
docker compose exec -T postgres createdb -U backup_probe restore_probe
docker compose exec -T postgres pg_restore -U backup_probe -d restore_probe --no-owner --no-privileges --exit-on-error < "$task_temp/archive/database.dump"
task_restored=$(docker compose exec -T postgres psql -X -U backup_probe -d restore_probe -Atc \
  "SELECT content FROM backup_probe_data")
[[ "$task_restored" = synthetic-backup-content ]] || { echo 'Restored database data differs.' >&2; exit 1; }
task_version=$(docker compose exec -T postgres psql -X -U backup_probe -d restore_probe -Atc \
  "SELECT version FROM schema_migrations WHERE NOT dirty")
[[ "$task_version" = 5 ]] || { echo 'Restored schema version differs.' >&2; exit 1; }
task_settings=$(docker compose exec -T postgres psql -X -U backup_probe -d restore_probe -Atc \
  "SELECT clinic_name || ':' || default_ticket_priority FROM application_settings")
[[ "$task_settings" = MedDesk:MEDIUM ]] || { echo 'Restored application settings differ.' >&2; exit 1; }
mkdir -m 700 "$task_temp/restored-media"
tar -xzf "$task_temp/archive/media.tar.gz" -C "$task_temp/restored-media"
[[ "$(cat "$task_temp/restored-media/probe.txt")" = synthetic-media ]] || { echo 'Restored media bytes differ.' >&2; exit 1; }
docker compose exec -T backend sh -ec 'test "$APP_ENV" = development' >/dev/null
echo 'PASS disposable database/media backup, checksums, restore and backend restart.'
