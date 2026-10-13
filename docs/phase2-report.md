# Phase 2 delivery report — identity and administration

Completed on 2026-10-09. Phases 1 and 2 are implemented; Phases 3–5 remain pending.

## Delivered behavior

The Go API now implements login, current session, logout, own display-name profile editing, and administrator user/department administration. The React interface provides Kazakh/Russian login, protected overview/profile, filtered/paginated administration tables, create/edit forms, toasts, and confirmation dialogs. Data comes from PostgreSQL.

Passwords use bcrypt cost 12. Opaque session/CSRF values are stored as hashes; HttpOnly cookies, exact Origin checks, session-bound CSRF headers, bounded login throttling, fresh database role/active checks, and account session revocation protect the implemented endpoints. Production configuration requires secure cookies, explicit HTTPS origins, and verified PostgreSQL TLS.

Management changes use transactions, current-actor rechecks, a shared advisory lock, row locks, and scrubbed audit events. Policies protect own/last administrator authority and prevent active users from belonging to inactive departments. Audit insertion failure rolls the mutation back. User/department records are deactivated rather than deleted.

Development provisioning creates three synthetic role accounts from externally supplied passwords only with explicit `APP_ENV=development`. Setup generates missing random passwords privately in ignored `.env`. Repeating the seed preserves existing account state. There is no automatic demo login or production demo bootstrap.

Phase 2 reuses migration 1's identity tables. The database schema version remains 1; query sources and generated sqlc code were extended without unnecessary schema changes.

## Changed and added files

- `backend/internal/auth/`: models, credential/session service, pgx/sqlc repository, cookie/CSRF/RBAC guard, HTTP handlers, rate limiter, service/handler/limiter tests.
- `backend/internal/users/`: DTOs/models, validation service, transactional repository, administration/profile handlers, service/handler tests.
- `backend/internal/departments/`: DTOs/models, service, repository, handlers, policy tests.
- `backend/internal/platform/httpx/request.go` and `request_test.go`: bounded strict JSON, text/UUID/filter/pagination validation.
- `backend/internal/config/config.go` and `config_test.go`: session/cookie/origin/proxy settings and fail-closed production validation.
- `backend/cmd/api/main.go`, `backend/internal/platform/health/handler.go`, `handler_test.go`: constructor wiring, feature route registration, Phase 2 metadata.
- `backend/db/queries/auth.sql`, `users.sql`, `departments.sql`; generated `backend/internal/platform/db/auth.sql.go`, `users.sql.go`, `departments.sql.go`, `querier.go`.
- `backend/tests/auth_integration_test.go`, `management_integration_test.go`, `identity_http_integration_test.go`: actual session/CRUD/transaction/concurrency and HTTP-to-PostgreSQL workflows.
- `backend/cmd/seed/main.go`, `backend/internal/seed/seed.go`, `seed_test.go`, `backend/db/seeds/embed.go`: atomic, idempotent development account seed.
- `backend/go.mod`, `go.sum`, `Dockerfile`: bcrypt dependency and API/seed binaries, version 0.2.0.
- `backend/api/openapi/openapi.json`: implemented session/profile/user/department contract, security requirements and errors.
- `frontend/src/lib/auth.tsx`, `api.ts`, `validation.ts`, `phase2-locale.ts`, `notifications.tsx`, `locale.tsx`: authenticated API/state, form validation, localized messages and notifications.
- `frontend/src/pages/Login.tsx`, `Profile.tsx`, `Users.tsx`, `Departments.tsx`, `Overview.tsx`; `frontend/src/App.tsx`, `components/Shell.tsx`: protected pages, role navigation and pending-feature states.
- `frontend/src/components/forms/FormField.tsx`, `Pagination.tsx`, `Modal.tsx`; `frontend/src/styles.css`: accessible administration forms/dialogs and responsive layout.
- `frontend/src/test/App.test.tsx`, `api.test.ts`; `frontend/package.json`, `package-lock.json`: authentication/component regressions and version 0.2.0.
- `.env.example`, `docker-compose.yml`, `Makefile`, `scripts/init-demo-env.sh`, `scripts/smoke-auth.py`: development configuration, explicit seed job and private authentication smoke tooling.
- `README.md`, `docs/architecture.md`, `requirements.md`, `database.md`, `implementation-plan.md`, this report: actual scope, operational instructions, decisions and verification.
- Ignored local `.env`: generated development demo password variables; existing database credentials preserved. Secrets are not included in the report or source code.

The existing CI workflow already runs the extended Go/frontend/integration checks. No remote CI execution is claimed.

## Executed verification

- `make test`: passed Go race tests, `go vet ./...`, frontend clean install/typecheck/tests/production build. PostgreSQL-only tests skip in this command without `TEST_DATABASE_URL`.
- `make test-integration`: passed the real PostgreSQL race-enabled suite **twice**, before and after disposable migration down/up. Includes five authentication repository tests, management persistence/filtering/revocation, audit-failure rollback, department policy, schema/search foundation, and the real HTTP identity/administration workflow. Reference seed idempotency and migration rollback were verified. Project database/volumes were untouched.
- After final config/seed changes: `go test -race ./internal/config ./internal/seed` and matching `go vet` passed.
- After final frontend fixes: `make test-frontend` passed clean install, TypeScript checks, **36 tests**, and production build.
- `make generate`: regenerated sqlc code with matching before/after file hashes. OpenAPI JSON/local references validated.
- `make compose-check`: passed without printing resolved environment credentials.
- `make up`: built images, applied migrations and started healthy PostgreSQL/backend/frontend/gateway. The final frontend image was rebuilt after the last UI regressions were fixed.
- `make seed-accounts`: created three accounts; repeating the seed created zero accounts and preserved existing state.
- Production-mode seed negative check: exited with the expected development-only rejection before database connection. `.env` permissions verified as 600; Bash syntax and Python smoke syntax checks passed.
- `make smoke`: all six public API/SPA probes returned 200.
- `make smoke-auth`: all three roles passed login/current session/cookie scope/HttpOnly checks, administrator isolation, missing CSRF and missing Origin rejection, logout and subsequent unauthorized access. No credentials or tokens were printed.
- Headless Chrome against the final Docker image: three real role logins/logout, own profile update/restore, user/department lists, employee administration denial, Russian localization, initial field focus/Escape/opener restoration, and desktop/mobile layouts passed without runtime/CSP errors or document overflow. Real UI dialogs created/edited/deactivated a temporary department and synthetic employee, including role-change confirmation; persisted API results were checked. Exact temporary fixtures were removed afterward, demo profile state restored, and scripted sessions logged out. Six representative screenshots were visually inspected. This browser smoke is a local developer check, not a committed CI browser suite.

## Issues found and fixed

- Email normalization could replace invalid UTF-8 before validation. Original input is now checked first.
- Logout could lock a session before its owner while account administration used the opposite order. The consistent user-before-session order is now verified by a real concurrent PostgreSQL regression.
- A race-enabled full HTTP test outgrew its shared 30-second fixture context due to repeated bcrypt work. Its whole-workflow test budget was increased; production request deadlines remain bounded at their configured value.
- Query caches and editor/profile state could outlive a cross-tab account/role change. Private caches clear, editor boundaries remount, and late profile responses cannot replace refreshed identity/role metadata. Stale CSRF failures clear authenticated state.
- Dialogs lacked opener focus restoration and initially focused the close button. Keyboard focus now enters the first enabled form field and returns to the opener on close.
- Exact-origin validation now rejects empty query/fragment delimiters, which would otherwise never match the browser's Origin.

## Verify locally

```bash
make up
make seed-accounts
make smoke smoke-auth
make test
make test-integration
```

Open `http://localhost:8080/login`. Use the three synthetic account emails and their private `DEMO_*_PASSWORD` values from `.env`. Employees/engineers can edit their own display name and cannot access administration APIs/pages. Administrators can create/filter/edit users and departments, reset another account's password, and verify its old session loses access. Try deactivating an occupied department and demoting your own administrator account; both must be rejected. Switch locale and exercise forms at mobile/desktop widths and using keyboard navigation.

Account/department mutations are persistent. Use synthetic records when verifying; integration tests supply disposable fixtures automatically. Changing the development seed password variables does not change existing stored passwords.

## Next phase

Phase 3 will implement Knowledge Base article/category/tag operations, publication, multilingual ranked search, views/feedback, and authenticated image/video upload/download. Ticket workflows are Phase 4; database-derived analytics and remaining administration experiences are Phase 5. Phase 2 completion does not mean the full MedDesk MVP or clinic production deployment is complete.
