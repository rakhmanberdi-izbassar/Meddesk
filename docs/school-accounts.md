# School employee accounts (migration 7)

School employees use the normal MedDesk login page, but can only submit and read their own school requests. The existing clinic account that owns the school remains usable in both workspaces. This is one school workspace, not a general multi-organization tenant system.

## Owner workflow

Open **Мектеп → Қызметкерлер → Қызметкер қосу** (Russian: **Школа → Сотрудники → Добавить сотрудника**). Enter the employee's name, unique email and initial password. Use the existing 12–72 UTF-8 byte password policy. Share initial credentials privately; the application does not send email or store recoverable passwords.

Existing emails cannot be reused for a second school account because login identities are globally unique. Clinic accounts are not automatically converted. School employee email and role are fixed after creation in this version; the owner can update the name, reset the password, disable and reactivate the account. Reset and deactivation require confirmation and revoke sessions atomically. Updates require the current account version.

Employees sign in through the same `/login` page and are redirected to `/school`. They enter title, description, room, category, priority and whether a lesson is blocked. Requester identity and display name come from the authenticated account, not submitted fields. They can read status and an employee-visible answer. They cannot see other employees' requests, device inventory, IP addresses, internal repair notes, account management or any clinic resources.

The owner sees all school requests and processes them in the existing school panel. **Ішкі техникалық жазба / Внутренняя техническая запись** is owner-only. **Қызметкерге жауап / Ответ сотруднику** is returned to the employee. Do not put private network details in the public reply. Legacy manually entered requests remain owner-only; matching a name does not grant an employee access.

## Identity and authorization

`users.account_scope` is `CLINIC` by default, preserving all existing accounts. `SCHOOL` accounts have no clinic department. A database constraint enforces scope/department shape. School creation fixes role to EMPLOYEE; no role, scope or department fields are accepted from the client.

Authentication queries return the scope on login and session introspection. The shared authorization guard rejects SCHOOL identities from every clinic handler, even if their role were ADMIN. It allows only school APIs, session introspection and logout; school services then independently enforce owner-only or employee-only permissions. Active membership is read under transaction locks, so deactivation and password reset serialize with login and in-flight school operations. Clinic account listing, direct lookup, mutation, administrator counts and engineer filters exclude school identities, independently of role.

School requests store an optional `requester_id` for authenticated employee submissions. Employee list/detail queries predicate on that ID and explicitly project safe fields: they never select internal resolution, device IDs or requester metadata. Another employee's request ID returns 404. Owner requests and device endpoints continue requiring explicit clinic-scope school ownership.

School account and authentication events are stored in `school_account_events`, outside clinic audit logs. Account changes, password hashes, session revocation and private audit events share transaction boundaries. No password/hash/session secret is returned by school member endpoints. Database operators and whole database backups remain trusted infrastructure boundaries.

## Deployment

1. Back up the database. Apply pending migrations through **000007_school_accounts** using the existing migration workflow before deploying the new backend. Existing clinic users and school ownership need no manual conversion.
2. Deploy backend and frontend from the same revision. `/api/v1/ready` requires clean schema version **7**.
3. Sign in with the already configured school owner's account and create school employees from the **school** employee tab. Do not use the clinic user-management page for them.
4. Verify an employee can submit and view a school request, another employee cannot see it, and `/api/v1/knowledge/articles` and `/api/v1/school/assets` are denied to the employee.

No new secret/environment setting is required. Apply migrations with the migration tool; do not force schema versions. Do not run the down migration on production. Migration 7 refuses rollback while SCHOOL users exist so that they cannot accidentally become clinic accounts. Rollback tests use disposable databases only.

## API additions

Existing cookie, Origin and CSRF rules apply. Owner-only endpoints:

- `GET /api/v1/school/members?limit=20&offset=0`
- `POST /api/v1/school/members` — email, display_name, password
- `PATCH /api/v1/school/members/{id}` — display_name, is_active, expected version, optional replacement password

Employee-only endpoints:

- `GET /api/v1/school/requests?limit=20&offset=0`
- `POST /api/v1/school/requests` — title, description, room, category, priority, lesson_blocked
- `GET /api/v1/school/requests/{id}` — own request only

`GET /api/v1/school/access` now returns `allowed` and `owner`. The owner ticket update endpoint accepts `public_reply` separately from internal `resolution`. See OpenAPI for exact schemas and errors.

## Verification commands

```bash
cd backend
go test -race ./internal/... ./cmd/...
go vet ./internal/... ./cmd/...
cd ../frontend
npm run build
npm test -- --maxWorkers=1
cd ..
bash scripts/test-integration.sh
```

Use one frontend test worker on constrained machines, especially while race-instrumented Go tests run. No assertion or timeout relaxation is required. PostgreSQL scenarios cover real scoped login, clinic denial, private device denial, cross-employee ID protection, safe response projections, private authentication audit, clinic user-management separation, password reset/deactivation, stale versions and account audit rollback.

Attachments, employee comments, self-service password changes, public registration and additional schools are not part of this delivery. The owner can reset employee passwords.
