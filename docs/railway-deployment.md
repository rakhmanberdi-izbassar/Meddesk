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
