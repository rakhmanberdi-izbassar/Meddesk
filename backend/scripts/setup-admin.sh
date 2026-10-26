#!/bin/sh
# Operator-only interactive wrapper; credentials never become command-line arguments.
set +x
set -eu
trap 'unset MEDDESK_ADMIN_PASSWORD MEDDESK_ADMIN_CONFIRM BOOTSTRAP_ADMIN_EMAIL' 0
printf 'Administrator email: '
IFS= read -r BOOTSTRAP_ADMIN_EMAIL
export BOOTSTRAP_ADMIN_EMAIL
printf 'Administrator password (12–72 UTF-8 bytes; hidden): '
IFS= read -r -s MEDDESK_ADMIN_PASSWORD
printf '\nRepeat password (hidden): '
IFS= read -r -s MEDDESK_ADMIN_CONFIRM
printf '\n'
if [ "$MEDDESK_ADMIN_PASSWORD" != "$MEDDESK_ADMIN_CONFIRM" ]; then
    printf 'Passwords differ; no account was created.\n' >&2
    exit 1
fi
printf '%s\n' "$MEDDESK_ADMIN_PASSWORD" | /app/bootstrap-admin -confirm-empty-database
