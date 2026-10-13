#!/usr/bin/env bash
# All migrations, including destructive rollback, run on a disposable DB only.
set -euo pipefail
cd "$(dirname "$0")/.."
task_container="meddesk-test-$(openssl rand -hex 6)"
task_password=$(openssl rand -hex 24)
task_migrate_image=migrate/migrate:v4.19.1
task_pg_image=postgres:18-alpine
cleanup() { docker rm -fv "$task_container" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run --detach --name "$task_container" \
  -e POSTGRES_USER=meddesk_test -e POSTGRES_DB=meddesk_test \
  -e "POSTGRES_PASSWORD=$task_password" \
  -p 127.0.0.1::5432 "$task_pg_image" >/dev/null
task_ready=false
for ((task_attempt=0; task_attempt<60; task_attempt++)); do
  if docker exec "$task_container" pg_isready -h 127.0.0.1 -U meddesk_test -d meddesk_test >/dev/null 2>&1; then
    task_ready=true
    break
  fi
  sleep 1
done
[[ "$task_ready" = true ]] || { echo 'Disposable PostgreSQL did not become ready.' >&2; exit 1; }
task_port=$(docker port "$task_container" 5432/tcp | sed 's/.*://')
task_container_url="postgres://meddesk_test:$task_password@127.0.0.1:5432/meddesk_test?sslmode=disable"
export TEST_DATABASE_URL="postgres://meddesk_test:$task_password@127.0.0.1:$task_port/meddesk_test?sslmode=disable"
run_migrate() {
  docker run --rm --network "container:$task_container" \
    -v "$PWD/backend/db/migrations:/migrations:ro" \
    "$task_migrate_image" -path=/migrations -database="$task_container_url" "$@"
}
echo 'Applying migrations to disposable PostgreSQL.'
run_migrate up
(cd backend && go test -race -count=1 -v ./tests/...)
echo 'Checking development reference seed idempotency.'
for task_seed_run in 1 2; do
  docker exec -i "$task_container" psql -X -U meddesk_test -d meddesk_test \
    --set ON_ERROR_STOP=1 < backend/db/seeds/development.sql >/dev/null
done
echo 'Rolling back and reapplying disposable schema.'
run_migrate down -all
task_tables=$(docker exec "$task_container" psql -X -U meddesk_test -d meddesk_test -Atc \
  "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'schema_migrations'")
[[ "$task_tables" = 0 ]] || { echo 'Rollback left application tables.' >&2; exit 1; }
run_migrate up
(cd backend && go test -race -count=1 -v ./tests/...)
echo 'PASS migration up/down/up, reference seed and repository integration checks.'
