#!/bin/sh
set -eu

# Only a DNS name and port may enter the Nginx configuration, never arbitrary directives.
: "${BACKEND_HOST:?Set BACKEND_HOST to the backend private DNS name}"
: "${BACKEND_PORT:?Set BACKEND_PORT to the backend HTTP port}"
if ! printf '%s' "$BACKEND_HOST" | grep -Eq '^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$'; then
    echo 'BACKEND_HOST must be a DNS hostname without scheme, path or credentials' >&2
    exit 1
fi
case "$BACKEND_PORT" in
    ''|*[!0-9]*) echo 'BACKEND_PORT must be an integer from 1 to 65535' >&2; exit 1 ;;
esac
if [ "${#BACKEND_PORT}" -gt 5 ] || [ "$BACKEND_PORT" -lt 1 ] || [ "$BACKEND_PORT" -gt 65535 ]; then
    echo 'BACKEND_PORT must be an integer from 1 to 65535' >&2
    exit 1
fi

# Railway supplies a private DNS resolver; do not hardcode Docker's resolver address.
DNS_RESOLVER=$(awk '$1 == "nameserver" {print $2; exit}' /etc/resolv.conf)
case "$DNS_RESOLVER" in
    ''|*[!0-9a-fA-F.:]*) echo 'No supported DNS resolver found' >&2; exit 1 ;;
esac
case "$DNS_RESOLVER" in *:*) DNS_RESOLVER="[$DNS_RESOLVER]" ;; esac
export DNS_RESOLVER BACKEND_HOST BACKEND_PORT

# Explicit substitution preserves Nginx variables such as $request_uri and $host.
umask 077
envsubst '${BACKEND_HOST} ${BACKEND_PORT} ${DNS_RESOLVER}' \
    < /etc/meddesk/nginx.conf.template > /tmp/meddesk-nginx.conf
nginx -t -c /tmp/meddesk-nginx.conf
exec nginx -c /tmp/meddesk-nginx.conf -g 'daemon off;'
