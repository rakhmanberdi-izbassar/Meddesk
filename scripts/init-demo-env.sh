#!/usr/bin/env bash
# Generate optional demo passwords locally without displaying them or evaluating .env.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f .env ]] || { echo 'Run make env first.' >&2; exit 1; }
task_environment=''
declare -A task_existing
while IFS= read -r task_line || [[ -n "$task_line" ]]; do
  [[ "$task_line" = \#* || "$task_line" != *=* ]] && continue
  task_key=${task_line%%=*}
  task_value=${task_line#*=}
  [[ "$task_key" = APP_ENV ]] && task_environment=$task_value
  case "$task_key" in DEMO_EMPLOYEE_PASSWORD|DEMO_ENGINEER_PASSWORD|DEMO_ADMIN_PASSWORD) task_existing["$task_key"]=$task_value ;; esac
done < .env
[[ "$task_environment" = development ]] || { echo 'Demo credentials require explicit APP_ENV=development.' >&2; exit 1; }
command -v openssl >/dev/null
chmod 600 .env
umask 077
task_temporary=$(mktemp .env.demo.XXXXXX)
trap 'rm -f "$task_temporary"' EXIT
cp .env "$task_temporary"
for task_role in EMPLOYEE ENGINEER ADMIN; do
  task_key="DEMO_${task_role}_PASSWORD"
  [[ -n "${task_existing[$task_key]:-}" ]] && continue
  task_password=$(openssl rand -hex 24)
  # Passwords are generated hex; neither user values nor secrets are evaluated.
  sed "/^${task_key}=/d" "$task_temporary" > "${task_temporary}.new"
  mv "${task_temporary}.new" "$task_temporary"
  printf '\n%s=%s\n' "$task_key" "$task_password" >> "$task_temporary"
done
mv "$task_temporary" .env
echo 'Development demo passwords are configured privately in .env; existing values preserved.'
