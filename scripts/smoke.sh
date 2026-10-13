#!/usr/bin/env bash
set -euo pipefail
task_base_url=${MEDDESK_URL:-http://127.0.0.1:8080}
for task_path in /api/v1/health /api/v1/ready /api/v1/system /api/v1/openapi.json / /login; do
  task_code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "$task_base_url$task_path")
  [[ "$task_code" = 200 ]] || { echo "FAIL $task_path returned $task_code" >&2; exit 1; }
  echo "PASS $task_path ($task_code)"
done
