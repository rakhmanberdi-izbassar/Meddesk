#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ -e .env ]]; then
  echo '.env already exists; leaving it unchanged.'
  exit 0
fi
command -v openssl >/dev/null || { echo 'openssl is required to generate local credentials.' >&2; exit 1; }
umask 077
task_password=$(openssl rand -hex 32)
# Values are generated hex, never shell-evaluated or printed.
sed -e "s/^POSTGRES_PASSWORD=$/POSTGRES_PASSWORD=$task_password/" \
  -e "s|^DATABASE_URL=$|DATABASE_URL=postgres://meddesk:$task_password@127.0.0.1:5432/meddesk?sslmode=disable|" \
  .env.example > .env
echo 'Created private .env with random local database credentials.'
