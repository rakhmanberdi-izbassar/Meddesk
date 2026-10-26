# Railway PostgreSQL TLS

MedDesk production requires `sslmode=verify-full`. A private PostgreSQL CA must be trusted by the **backend**, and the server certificate must cover the database hostname. Adding the CA only inside the Postgres container does not configure the backend. Public API HTTPS and database TLS are independent.

The optional backend variable `DATABASE_CA_CERT` accepts a multiline public CA certificate bundle in PEM format. When present, the bundle supplies the database trust roots; full chain/hostname verification remains enabled and plaintext fallbacks are rejected. Invalid certificate bundles stop startup without logging their contents or database credentials. Without this variable, the existing pgx certificate configuration remains in effect. Never supply a private key.

For the observed Railway Postgres certificate paths, run in **Postgres → Console** (shell, not `psql`):

```bash
openssl verify \
  -CAfile /var/lib/postgresql/data/certs/root.crt \
  -verify_hostname "$PGHOST" \
  /var/lib/postgresql/data/certs/server.crt
```

Only continue if this succeeds. A hostname mismatch requires correcting the server certificate/DNS configuration; importing a CA does not fix it. The command verifies the local certificate files; backend connectivity still needs its own readiness check.

Copy the public CA with:

```bash
cat /var/lib/postgresql/data/certs/root.crt
```

Set **Meddesk → Variables → DATABASE_CA_CERT** to that entire multiline value, including the BEGIN/END CERTIFICATE lines. Keep `DATABASE_URL` pointed to the intended database with `sslmode=verify-full`, `APP_ENV=production`, `COOKIE_SECURE=true`, and the exact HTTPS UI origin in `AUTH_ALLOWED_ORIGINS`. Do not copy `root.key` or `server.key`. Do not commit certificates or connection credentials into the repository. Publish/deploy the backend code containing this variable before expecting it to take effect.

Apply migrations 1–4 to that database using the existing migration runner. Then `/api/v1/ready` must return `{"status":"ready"}`. A successful API readiness check does not deploy the React UI: frontend and same-origin `/api` routing still require configuration. Persistent media storage, initial administrator provisioning and backup policy are separate deployment tasks. Rotate any previously exposed database password on the actual database and update connection references together.

## React frontend and same-origin API gateway

The standard frontend image intentionally returns 404 for `/api/` because local Compose has a separate gateway. Railway should instead build `frontend/Dockerfile.railway`, which serves the SPA and proxies `/api/` to the backend's private HTTP endpoint. Cookies, uploads and API calls share the frontend's public HTTPS origin; no cross-origin API URL or wildcard CORS is needed.

After publishing these files, add a **new** GitHub-backed service named `meddesk-web` in the same Railway project/environment:

1. Select the existing repository/branch and set **Root Directory** to `/frontend`.
2. Set `RAILWAY_DOCKERFILE_PATH=Dockerfile.railway`. Leave custom build/start commands unset; the image supplies them.
3. Set `BACKEND_HOST=${{Meddesk.RAILWAY_PRIVATE_DOMAIN}}` (replace `Meddesk` if its actual service name differs).
4. Set `BACKEND_PORT` to the backend's actual `HTTP_ADDR` port, e.g. `8081` for `:8081`; set frontend `PORT=8080`.
5. Generate a public domain for **meddesk-web**, target port **8080**. Do not replace the existing backend domain while validating the new service.
6. Set the **backend** `AUTH_ALLOWED_ORIGINS` to the exact generated frontend HTTPS origin (no trailing slash). Multiple deliberate origins are comma-separated. Keep `COOKIE_SECURE=true` in production; do not change database credentials or the CA certificate.
7. Deploy both pending configurations. On the frontend domain, verify `/healthz`, `/login`, `/api/v1/health` and `/api/v1/ready`. The last must return `{"status":"ready"}`. Unmatched `/api/` URLs must remain API responses rather than the SPA.

The proxy discovers DNS from the container resolver, supports IPv4/IPv6 private DNS, refreshes backend resolution, and streams authorized media through the API. It does not publish media as static files. Do not supply database passwords to the frontend service. Backend client-IP throttling continues using the default untrusted-proxy policy until actual Railway/gateway proxy trust is explicitly validated; do not allow global proxy CIDRs. This frontend gateway forwards its immediate peer address and does not treat arbitrary forwarded client headers as authenticated identity.

The existing production seed intentionally rejects development demo accounts. A login page alone does not provision a production administrator. Use the one-time bootstrap below and keep the validated backend and database running. Persistent backend media storage remains required before real clinic use.

## First production administrator

The backend image includes `/app/bootstrap-admin`. It requires explicit `-confirm-empty-database`, `APP_ENV=production` and a reachable database with clean migrations. It uses the same database URL/CA configuration as the API. The email comes from `BOOTSTRAP_ADMIN_EMAIL`; the password arrives only on stdin, not as a CLI argument. Optional `BOOTSTRAP_ADMIN_NAME` and `BOOTSTRAP_ADMIN_DEPARTMENT` default to `Әкімші` and `Әкімшілік`.

After publishing the updated Dockerfile/command and deploying the **backend**, open **Meddesk → Console**, not the Postgres or frontend console. Enter the following commands one step at a time. Do not use a shell with command tracing enabled. Choose your own email and a unique 12–72 UTF-8 byte password; never send the password in chat or store it in source/Railway variables.

For the simplest interactive path, run `sh /app/setup-admin.sh` and answer its email, hidden password and repeat-password prompts. It checks that both password entries match and passes the password only on stdin to the guarded CLI. The manual equivalent is below.

```sh
printf 'Administrator email: '
read -r BOOTSTRAP_ADMIN_EMAIL
export BOOTSTRAP_ADMIN_EMAIL
read -rs -p 'Administrator password (12–72 bytes): ' MEDDESK_ADMIN_PASSWORD
printf '\n'
read -rs -p 'Repeat password: ' MEDDESK_ADMIN_CONFIRM
printf '\n'
if [ "$MEDDESK_ADMIN_PASSWORD" = "$MEDDESK_ADMIN_CONFIRM" ]; then
  printf '%s\n' "$MEDDESK_ADMIN_PASSWORD" | /app/bootstrap-admin -confirm-empty-database
else
  printf 'Passwords differ; no account was created.\n'
fi
unset MEDDESK_ADMIN_PASSWORD MEDDESK_ADMIN_CONFIRM BOOTSTRAP_ADMIN_EMAIL
```

Success prints `Initial administrator created.` Sign in at the frontend `/login` with the supplied email/password. Use user management for subsequent employees/engineers. This CLI creates exactly one initial administrator and the named active department (or reuses an existing active department), with bcrypt cost 12 and transactional audits. A shared administration advisory lock plus a users-table lock protects the empty check and inserts. It refuses whenever **any** user already exists, including inactive users; it never resets passwords or promotes existing users. An inactive matching department is rejected. Audit failure rolls back the user/new department. A failed or interrupted commit requires checking stored state before retrying; bootstrap is not a password-recovery tool.

Local verification passed: bootstrap input/stdin/confirmation unit tests under the Go race detector, focused vet, a production backend Docker build including the CLI, and three real PostgreSQL 18 race-enabled tests (first account plus repeat refusal/credential preservation, inactive department refusal, audit-failure rollback). The disposable database was removed. No production administrator was created by these local checks. Reproduce the isolated database checks with `python3 scripts/test-bootstrap.py` from the repository root (Docker, Go and Python 3 required), or run the existing full `make test-integration` suite. Concurrent invocation is protected by database locks but a separate concurrent-bootstrap test has not been executed.

### Executed local checks

The Railway frontend image passed a production TypeScript/Vite build and a real Docker smoke against a disposable synthetic API: non-root startup, SPA deep links and assets, readiness/API 404 routing, POST/query/Origin/cookie/CSRF header forwarding, media byte ranges, security headers and rejection of invalid host/port configuration. The first smoke exposed duplicate Nginx proxy timeout directives; these were moved to the enclosing HTTP scope and the corrected smoke passed. All temporary smoke containers/network were removed. These results do not claim a live Railway frontend deployment, real user login or browser acceptance.

Reproduce from the repository root with Docker and Python 3:

```bash
docker build -f frontend/Dockerfile.railway -t meddesk-web-railway:local frontend
python3 scripts/test-railway-frontend.py
```
