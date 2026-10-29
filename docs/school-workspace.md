# Private school workspace

The `/school` workspace is a private engineering notebook for the school, including network and surveillance incidents. The existing clinic portal remains separate. One explicit clinic account owns one school workspace; migration 7 adds scoped school employee accounts. This is not a general multi-tenant conversion. Clinic ADMIN and IT_ENGINEER roles do not grant school access.

## Included

- Register school requests received by phone or in person, with requester, room, category, priority and lesson disruption.
- Categories: computers, printing, network/Wi-Fi, surveillance, classroom equipment, accounts and other.
- Maintain devices: computers, printers, cameras, recorders, routers, switches, projectors and other equipment. Store location, model, unique nonempty inventory number, optional IPv4/IPv6 address and notes.
- Link a request to a school device by searching its name, room or inventory number. From a device, open its requests; this filter and pagination run on the server. The selected device is prefilled when creating a request from its history.
- Process requests: NEW → IN_PROGRESS → WAITING_FOR_USER / RESOLVED → CLOSED. Waiting requests may resume or resolve; resolved requests may reopen. Resolution text is mandatory to resolve/close. Closed requests are final. Priority may change while processing.
- Every edit uses optimistic version checks. Record changes and private history commit atomically.
- Kazakh and Russian interface labels. User-entered incident descriptions are preserved as entered.

School employee login and own-request submission are now available through migration 7; see [school accounts](school-accounts.md). Public registration, public submission links, school knowledge articles, attachments, camera feeds, monitoring, password vaults and deletion are not provided. The owner can also record requests received by phone.

## Access and isolation

Migration 000006 creates `school_workspace`, `school_assets`, `school_tickets` and `school_history`. It never auto-grants an owner. No school record is inserted into clinic tickets, departments, knowledge articles, analytics, attachments or audit exports. Private ticket history stores actor/entity/action/version/time without ticket bodies or network details. School account/authentication history is also private.

Owner operations check the active account against the explicitly configured owner inside a database transaction. Employee operations check active SCHOOL scope and restrict requests to the authenticated employee. Mutation authorization locks the ownership and user rows so revocation waits for in-flight operations and blocks subsequent ones. Sidebar visibility is a convenience; the API independently enforces ownership. Even another clinic administrator gets HTTP 403.

Clinic user management also refuses other administrators' changes to the school owner's account, including password resets, email/role changes and deactivation. An administrator who owns the workspace can manage their own account under existing account rules. Lost-owner recovery and ownership transfer require a trusted database operator. Database operators and full database backups necessarily have access to all stored records; this boundary does not isolate against the infrastructure operator.

Do not enter passwords, camera footage, patient information or unnecessary personal details. Devices and incident descriptions are operational metadata.

## Deploy and activate

1. Back up the database. Apply pending migrations through **000007** with the existing migration command before deploying the new backend. Deploy both backend and frontend. Readiness requires clean version **7**.
2. In Railway **Postgres → Console**, open PostgreSQL (the following command runs in the shell):

```bash
psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"
```

3. At the `railway=#` prompt, configure the exact existing owner's account. The prompt asks for the email, not a password. Do not replace the email with a shared clinic account.

```sql
\prompt 'School owner account email: ' school_owner_email
BEGIN;
INSERT INTO school_workspace (owner_id)
VALUES ((SELECT id FROM users WHERE lower(email) = lower(:'school_owner_email') AND is_active AND account_scope='CLINIC'));
SELECT u.email FROM school_workspace w JOIN users u ON u.id = w.owner_id;
COMMIT;
```

An unknown/inactive email fails the NOT NULL constraint; an already configured owner fails the singleton constraint. Both leave existing ownership unchanged. If a transaction fails interactively, run `ROLLBACK;` before another attempt. This deliberately does not silently transfer ownership.

4. Reload the owner's session page. The sidebar shows **Мектеп · жеке кеңістік / Школа · личное пространство**. Other clinic users see no school link. School employees see only their own-request portal. Check `/api/v1/ready` returns `{"status":"ready"}`.
5. Add a device and a test request. Verify another clinic admin cannot access `/school` or `/api/v1/school/tickets`. School requests must not appear in clinic ticket lists or analytics.

To revoke access, a trusted database operator can execute `DELETE FROM school_workspace;`. This retains all school records. Do not run the down migration on production; it deletes school data.

## API

All routes use the existing session cookie; mutations require Origin and CSRF protection.

- `GET /api/v1/school/access` → `{ "allowed": boolean }` for an authenticated user.
- `GET /api/v1/school/assets?limit=20&offset=0&q=Camera` → page of private devices.
- `GET /api/v1/school/assets/{id}` → one private device.
- `POST /api/v1/school/assets`, `PATCH /api/v1/school/assets/{id}` → device; PATCH requires `version`.
- `GET /api/v1/school/tickets?limit=20&offset=0&status=NEW&asset_id=<uuid>` → filtered request page. Status and asset filters are optional.
- `POST /api/v1/school/tickets` → new request; optional `asset_id` must reference school equipment.
- `PATCH /api/v1/school/tickets/{id}` → status, priority, resolution and expected version.

See OpenAPI for fields. Limits are 1–100; maximum offset is 1,000,000. Errors use existing structured responses, including forbidden (403), invalid_input (400), version_conflict (409) and not_found (404).

## Verification

```bash
cd backend
go test -race ./internal/school ./internal/users ./internal/platform/health
go vet ./internal/school ./internal/users ./cmd/api
cd ../frontend
npm test -- --run src/test/School.test.tsx
npm run build
cd ..
bash scripts/test-integration.sh
```

The integration script exclusively uses a disposable PostgreSQL container, tests migrations up/down/up, and covers explicit ownership, clinic-admin denial, owner-account password reset prevention, device-linked history, stale updates, workflow rules, clinic record separation, transactional history failure and revocation.
