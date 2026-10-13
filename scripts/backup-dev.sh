#!/usr/bin/env bash
# Coordinated private backup of the development Compose database and media.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
[[ $# -le 1 ]] || { echo 'Usage: bash scripts/backup-dev.sh [new-directory]' >&2; exit 1; }
docker compose exec -T backend sh -ec 'test "$APP_ENV" = development' >/dev/null 2>&1 || {
  echo 'Backup requires a running APP_ENV=development backend.' >&2
  exit 1
}
task_backup_dir=${1:-"$PWD/backups/$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"}
mkdir -p -- "$(dirname "$task_backup_dir")"
mkdir -m 700 -- "$task_backup_dir"
task_restart=false
task_complete=false
cleanup() {
  if [[ "$task_restart" = true ]]; then
    docker compose start backend >/dev/null 2>&1 || echo 'Backend restart failed; run docker compose start backend.' >&2
  fi
  if [[ "$task_complete" != true ]]; then
    echo 'Backup incomplete. Do not use this directory for recovery.' >&2
  fi
}
trap cleanup EXIT
task_restart=true
docker compose stop -t 30 backend >/dev/null
docker compose exec -T postgres sh -ec 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --no-owner --no-privileges' > "$task_backup_dir/database.dump"
docker compose run --rm --no-deps --entrypoint sh backend -ec 'exec tar -C /var/lib/meddesk/media -czf - .' > "$task_backup_dir/media.tar.gz"
(
  cd "$task_backup_dir"
  sha256sum database.dump media.tar.gz > checksums.sha256
)
printf '%s\n' 'MedDesk development backup; database and private media; keep encrypted and access-restricted.' > "$task_backup_dir/README.txt"
docker compose start backend >/dev/null
task_restart=false
task_complete=true
printf 'PASS development backup: %s\n' "$task_backup_dir"
