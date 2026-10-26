# MedDesk

Internal IT HelpDesk and Knowledge Base for a medical clinic. Go modular monolith, PostgreSQL, and a Kazakh/Russian React interface.

**Current milestone: Phase 5 complete and verified.** Authentication, administration, Knowledge Base, private media, support tickets, database-derived analytics, administrator audit browsing and persisted clinic settings work against PostgreSQL. Role dashboards display actual totals; no statistics are simulated. See the [Phase 5 delivery report](docs/phase5-report.md) for executed verification and its limits.

Version 0.5.1 adds staff Excel/CSV ticket exports for leadership and optional creation-date filters. See the [Kazakh usage guide](docs/ticket-export.md).

## Start locally

Requirements: Docker Engine with Compose v2+, Make, Bash, OpenSSL, and available port 8080. Python 3 is needed only for the authentication smoke command.

```bash
make env            # ignored .env with a random local database password
make up             # build, migrate, start, and wait for healthy containers
make seed-accounts  # generate private demo passwords and seed development accounts
make smoke
make smoke-auth
```

Open [MedDesk](http://localhost:8080). Kazakh is the default; select Russian in the top bar. Demo accounts are `employee@meddesk.test`, `engineer@meddesk.test`, and `admin@meddesk.test`. Their passwords are the corresponding `DEMO_EMPLOYEE_PASSWORD`, `DEMO_ENGINEER_PASSWORD`, and `DEMO_ADMIN_PASSWORD` values in your local `.env`. Setup generates random passwords without printing or committing them.

`make seed-accounts` requires explicit `APP_ENV=development` and creates synthetic departments/categories plus three safe Kazakh/Russian troubleshooting guides. Repeating it preserves existing passwords, roles, activation states, and article content/tags. Editing a demo password in `.env` after seeding does **not** reset its stored password; use the administrator user editor to reset it. `make seed-dev` seeds only reference departments/categories. API startup never creates demo accounts automatically.

Use `make logs` for application logs and `make down` to stop containers. Named PostgreSQL and private media volumes persist across shutdown/rebuild. `docker compose down --volumes` deletes local data; use it only to intentionally reset a disposable instance.

If port 8080 is occupied, change both `GATEWAY_PORT` and `AUTH_ALLOWED_ORIGINS` in `.env`, then run `make up`. For port 8090, allow `http://localhost:8090,http://127.0.0.1:8090` and run `MEDDESK_URL=http://127.0.0.1:8090 make smoke smoke-auth`. Authentication rejects origins outside the allowlist.

`make env` preserves existing `.env`. When creating it manually from `.env.example`, supply a random URL-safe `POSTGRES_PASSWORD` and matching host-development `DATABASE_URL`. Compose builds its connection string using internal PostgreSQL DNS; keep database credentials URL-safe. PostgreSQL initializes credentials on first volume creation; changing `.env` does not rotate an existing database password.

Development uses HTTP bound to loopback. A clinic production deployment requires HTTPS, secret management, tested backups/retention, separate migration/runtime database privileges, and controlled initial administrator provisioning. Production configuration rejects insecure cookies, implicit/non-HTTPS origins, and PostgreSQL connections without `sslmode=verify-full`. Default Compose PostgreSQL is for development.

For a PostgreSQL server signed by a private CA, set the backend's optional multiline `DATABASE_CA_CERT` to the public CA PEM. Full certificate/hostname verification remains required. See [Railway PostgreSQL TLS setup](docs/railway-deployment.md) for certificate checks, variable setup and readiness verification. This requires deploying the backend code that supports the variable; it does not publish the frontend automatically.

For Railway frontend hosting, use the dedicated `frontend/Dockerfile.railway` with a private backend host/port. It serves React and proxies `/api` under one public origin. Follow the [frontend deployment steps](docs/railway-deployment.md#react-frontend-and-same-origin-api-gateway); the standard frontend image is designed to sit behind the local Compose gateway.

For an empty production database, the backend image includes an operator-only `/app/bootstrap-admin -confirm-empty-database` command. It reads the password from stdin, preserves production TLS, refuses any existing users and commits the initial administrator/department/audits atomically. Follow the [first administrator instructions](docs/railway-deployment.md#first-production-administrator). Development demo provisioning remains disabled in production.

## Available behavior

- Employees and IT engineers can sign in, access the dashboard/profile, and edit their own display name.
- Administrators can search/filter/page users and departments, create accounts, change roles, reset passwords, and deactivate/reactivate records through confirmation dialogs.
- Account role/password/email/activation changes revoke existing sessions. Disabled accounts cannot sign in. Active users require active departments; departments with active users cannot be deactivated.
- Administrators cannot deactivate/demote themselves. Management transactions protect the last active administrator and commit scrubbed audit records together with mutations.
- All roles can search published guides in active categories, read ordered steps and authenticated screenshots/videos, and record helpful/not-helpful outcomes. Helpful feedback records self-service resolution without creating a ticket.
- Engineers/administrators can manage drafts/publication, tags, screenshots and videos. Article updates/deletes require the editing version; conflicts preserve form input. Administrators manage localized categories.
- All roles can submit and track their own support tickets. Employees see only their tickets and public comments. Engineers/administrators can filter all tickets, assign active staff, set priorities, add internal comments and manage the documented status transitions.
- Ticket changes compare versions; stale writes return a conflict. History and scrubbed audit metadata commit with each mutation. A requester can confirm closure of a resolved ticket; only administrators explicitly reopen closed tickets. Staff can convert a resolved/closed ticket once into a reviewed, unpublished guide.
- Engineers/administrators download filtered tickets from `/tickets` as Excel or CSV, across all pages (up to 5000 tickets). Excel includes the clinic, applied filters, current-state totals and average resolution hours. Creation-date filters are optional; no dates includes all history.
- Engineers/administrators use `/analytics` for date/timezone and department/category/assignee filters, real ticket/state totals, current-cohort resolution averages, self-service feedback ratios, category/department rankings, popular guides and daily charts with table alternatives.
- Administrators browse immutable `/admin/audit` events with bounded filters/pagination and action-specific typed metadata scrubbing. `/admin/settings` manages clinic name, new-ticket default priority and reporting timezone with optimistic versions, confirmation and atomic audits. Deployment secrets/security configuration stay outside these preferences.

## Authentication and security

Passwords use bcrypt cost 12 and require 12–72 UTF-8 bytes. Random opaque session and CSRF values are stored only as SHA-256 hashes in PostgreSQL. Both cookies are HttpOnly, SameSite=Lax, scoped to `/api/v1`, and Secure in production. Sessions expire after `SESSION_TTL` (default 8h, maximum 24h); login rotates the current session and logout revokes it. Protected operations re-read active account status and role.

The frontend obtains its CSRF token from login or `/auth/me` and keeps it in memory. Login requires an exact allowed `Origin`; authenticated mutations require that origin plus the session-bound CSRF cookie and `X-CSRF-Token`. Authentication uses the same-origin Nginx/Vite proxy; the API does not enable cross-origin access.

Login limits are 100 attempts per client address per 5 minutes and 10 per normalized account per 15 minutes. The bounded in-process limiter serves the single API instance. `TRUSTED_PROXY_CIDRS` defaults empty: forwarded headers are ignored, and the gateway shares an address-based limit. Configure only the actual trusted proxy network/address when deploying; global CIDRs are rejected.

JSON requests are bounded to 64 KiB, require `application/json`, reject unknown fields, and receive server-side validation. SQL is parameterized through sqlc. HTTP logs omit bodies, query values, email addresses, passwords, tokens, and cookies. Uploads and downloads require authentication; private file paths are never exposed as a static Nginx directory.

## Knowledge and media operations

The employee Knowledge Base is `/knowledge`; article details use `/knowledge/articles/{id}`. Staff management/editors use `/engineer/knowledge`; category management is `/admin/categories`. Searches use weighted PostgreSQL `simple` and `russian` vectors with literal substring/tag and trigram fallback for Kazakh/Russian queries. Views deduplicate per user/article within 15 minutes. Feedback uses an immutable attempt UUID; identical retries do not inflate counts, and changing the answer for that attempt returns 409.

Employees can upload ticket screenshots using `TICKET_ATTACHMENT`; article image/video uploads require staff. Uploads accept PNG/JPEG images up to 10 MiB, at most 10000 pixels per side/40 megapixels, and MP4/WebM videos up to 100 MiB by default. Images are fully decoded after dimension checks. Video container structures/tracks are validated; codec decoding, transcoding and malware scanning are not provided. Use browser-compatible video encodings. The frontend caps videos at 100 MiB; backend `MEDIA_MAX_VIDEO_BYTES` can impose a different limit within 1–500 MiB.

File bytes stream to private disk, with generated storage keys, SHA-256 metadata, anchored directory operations and symlink/path rejection. Active uploaders and administrators retain access independently of publication. Other engineers can read article-attached media and ticket attachments; ticket requesters can read their attachments. Other employees require published active-category article content and cannot read another requester’s ticket media. Native image/video requests use session cookies. Range/HEAD/conditional processing happens after authorization, with private/no-store responses.

JSON keeps its normal request deadline. Media upload and stream I/O use `MEDIA_UPLOAD_TIMEOUT` (default 2m) and `MEDIA_STREAM_TIMEOUT` (default 5m) without JSON timeout buffering. Nginx forwards uploads/responses without buffering; backend file limits remain authoritative. After editing mounted Nginx configuration, run `docker compose exec -T nginx nginx -t` and then `docker compose exec -T nginx nginx -s reload`.

Rejected/oversized/cancelled uploads and definite metadata failures remove partial files. A crash or uncertain database commit can leave private orphan files; retaining them prevents deleting a possibly committed reference. Article replacement/deletion does not delete media bytes automatically. There is no media-delete or automated retention/reconciliation job yet. Before clinic deployment, define retention and reconcile metadata/storage with backups; never delete referenced files blindly. See [database/operations details](docs/database.md).

## Ticket operations

Open `/tickets` for a filtered list, `/tickets/new` for the suggestion-first form, and `/tickets/{id}` for discussion/history. Helpful guide feedback ends the form without creating a ticket; unresolved problems retain the selected guide references and continue to creation. Requester/department come from the current authenticated account. Categories must be active and related guides published when the ticket is created. Attachments accept owned ticket-purpose PNG/JPEG screenshots only; administrators may select another uploader’s screenshot.

Transitions are `NEW → IN_PROGRESS`, `IN_PROGRESS → WAITING_FOR_USER|RESOLVED`, `WAITING_FOR_USER → IN_PROGRESS|RESOLVED`, and `RESOLVED → IN_PROGRESS|CLOSED`. Reopening clears the resolution timestamp. Closed tickets reject comments, attachments and ordinary updates; only an explicit administrator reopen can resume work. Draft conversion is a separate staff operation, supports resolved/closed tickets, and never publishes or copies screenshots automatically. Review the text before publication. Ticket versions also advance on comments, attachments and conversion. Reload after a conflict before retrying; unsaved text is preserved by the interface.

To change status, sign in as an engineer/administrator, open the ticket and use the status section's **«Жаңа күй» → «Күйді өзгерту»** controls. The employee role does not have these controls. A new ticket must first enter **«Орындалуда»**, then **«Шешілген»**.

On `/tickets`, apply the search/status/priority/category/department/assignee filters and optionally both creation dates, then choose **«Excel жүктеу»** or **«CSV жүктеу»**. Applied filters cover the whole selection, not just the current page. Date boundaries are inclusive local calendar dates in the clinic reporting timezone (or the chosen IANA timezone), up to 366 days; an empty date range includes all history. The API rejects a selection over 5000 tickets or a serialized file over 20 MiB rather than truncating it. Excel uses typed numbers/dates and inert text; CSV has a UTF-8 BOM and escapes spreadsheet formulas. Neither format includes credentials, email addresses, descriptions, comments or attachments. See the [export usage and report definitions](docs/ticket-export.md).

## Analytics, audit and settings

Reports use inclusive local dates (maximum 366 days), converted to an exclusive UTC end instant using calendar arithmetic. The default is the last 30 local days in the configured IANA timezone (`Asia/Almaty` initially). Ticket totals use tickets created during the selected interval and their current status: open is NEW/IN_PROGRESS/WAITING_FOR_USER; resolved includes RESOLVED/CLOSED; closed is its own subset. Average resolution spans original creation to current `resolved_at` for currently resolved/closed tickets. Reopened open tickets are excluded; a later re-resolution uses the original creation time. Daily resolved bars use creation-day cohorts, not resolution-day events.

Self-service rate is helpful completed immutable feedback attempts divided by all completed feedback attempts in the selected period; no attempts produces an absent measurement. Views alone do not count as feedback or ticket avoidance. Article views and feedback are aggregated separately. Department filtering of feedback/views uses the user’s current department, and category filtering uses the article’s current category; those historical membership snapshots were not stored in earlier phases. Assignee filtering applies to tickets only and does not alter self-service or guide-event totals. Common issues rank ticket category buckets, including uncategorized tickets. Reports/catalogs include inactive historical records for staff. One report uses a read-only repeatable-read PostgreSQL snapshot.

All authenticated roles can read safe clinic settings. Administrators edit them through the versioned form; stale edits preserve input until an explicit refresh. New ticket forms capture the initial configured priority without replacing a user choice on later refreshes. If settings are unavailable, the form explicitly shows the MEDIUM fallback and submits the displayed choice. API creation with an omitted priority reads the current clinic default within the creation transaction. Seed commands never reset these settings.

Read [operations and recovery](docs/operations.md). `make backup` briefly stops the development API and creates a private database/media backup; `make test-backup` verifies recovery on disposable synthetic services only. Neither command is a production deployment flow.

## Native development

Install Go 1.27.2 (or allow automatic Go toolchain selection), Node 24+, npm, and Docker. Dependencies are locked.

```bash
make env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres
make COMPOSE='docker compose -f docker-compose.yml -f docker-compose.dev.yml' migrate
make COMPOSE='docker compose -f docker-compose.yml -f docker-compose.dev.yml' seed-accounts

# Terminal 1, repository root (only source your own trusted .env):
set -a
source .env
set +a
export MEDIA_DIR=./var/media
cd backend
go run ./cmd/api

# Terminal 2:
cd frontend
npm ci
npm run dev
```

Vite serves `http://localhost:5173` and proxies `/api` to `127.0.0.1:8081`. The development Compose override exposes PostgreSQL only at `127.0.0.1:5432`; choose another port and update host `DATABASE_URL` if occupied. Use this override only for local development.

## Verification

```bash
make test                # Go race tests + vet; frontend typecheck/tests/build
make test-integration    # isolated PG18: migration up/down/up + real SQL/HTTP workflows
make test-backup         # disposable coordinated database/media backup and restore
make generate            # pinned sqlc Docker generator
make compose-check       # validate Compose without printing credentials
make smoke               # public HTTP checks against the gateway
make smoke-auth          # role sessions, knowledge search/visibility, CSRF and logout
```

`make test` requires host Go and Node; Docker builds do not. Integration tests require Docker, Go, and OpenSSL. They create a disposable database with random credentials/port and remove it afterward. Direct repository tests skip without `TEST_DATABASE_URL`; `make test-integration` supplies it and exercises PostgreSQL. Tests must use synthetic disposable data.

Coverage includes session rotation/expiry/revocation, concurrent login/account and logout/password-reset locking, rollback if audit insertion fails, department policy, a real HTTP login→admin CRUD→profile→role/password/activation→logout workflow, and composed upload→draft→publication→private ranges→feedback→category visibility→deletion checks. Authentication smoke reads demo passwords privately from generated `.env`, sends them only to loopback, and performs no profile/administrative/ticket changes. It verifies ticket ownership/internal-comment visibility, staff filter catalogs, ticket export permissions/files and published guide suggestions.

CI runs Go/frontend checks, PostgreSQL integration checks, sqlc regeneration consistency, and Compose validation. See [Phase 1](docs/phase1-report.md), [Phase 2](docs/phase2-report.md), [Phase 3](docs/phase3-report.md), [Phase 4](docs/phase4-report.md), and [Phase 5](docs/phase5-report.md) verification reports. Local runs do not imply remote GitHub Actions has executed.

## API and database tooling

[OpenAPI JSON](http://localhost:8080/api/v1/openapi.json) describes implemented operations and can be imported into Swagger Editor/Postman. Its source is [openapi.json](backend/api/openapi/openapi.json).

- Public: `GET /api/v1/health`, `/ready`, `/system`, `/openapi.json`; `POST /api/v1/auth/login`.
- Authenticated: `GET /api/v1/auth/me`, `POST /api/v1/auth/logout`, `PATCH /api/v1/profile`.
- Administrator: user/department operations and `POST/PATCH /api/v1/knowledge/categories`.
- Knowledge: authenticated articles/categories/tags under `/api/v1/knowledge`; article UUID/slug detail, filtered search/list, views and attempt feedback. Staff create/update/delete articles; publication visibility and version conflicts are enforced by the server.
- Reporting: staff `GET /api/v1/analytics` and `/analytics/filters`; administrator `GET /api/v1/admin/audit` and `/admin/audit/filters`.
- Settings: authenticated `GET /api/v1/settings`, administrator versioned `PATCH /api/v1/settings`.
- Tickets: `GET/POST /api/v1/tickets`, authorized details, comments, screenshots, requester confirmation, staff updates/assignees/draft conversion, and explicit administrator reopening. `/tickets/suggestions` searches published guides without creating a ticket.
- Ticket export: staff `GET /api/v1/tickets/export?format=xlsx|csv&locale=kk|ru` uses the same list filters plus optional `from`, `to`, `timezone`. Defaults are XLSX, Kazakh and all history; pagination parameters are rejected.
- Media: `POST /api/v1/media?purpose=ARTICLE_IMAGE|ARTICLE_VIDEO|TICKET_ATTACHMENT` with exactly one multipart `file`; employees may upload only ticket screenshots. Authenticated `GET/HEAD /api/v1/media/{id}` supports byte ranges and conditional requests.

Errors contain a stable code/message and request ID. Readiness requires a reachable database and expected clean schema. Public operational endpoints contain no employee/credential information.

Migrations live in `backend/db/migrations`; `make migrate` applies pending up migrations with golang-migrate. Phase 2 reused migration 1. Phase 3 adds migration 2 for article versions and a view-deduplication index. Phase 4 adds migration 3 for one draft conversion per ticket; Phase 5 adds migration 4 for clinic settings and a ticket analytics index; Migration 5 adds optional Russian article translations; readiness requires clean version 5. See [article localization](docs/article-localization.md) for editing and deployment. Existing development data survives these additive migrations. Dirty migrations require investigation/repair rather than automatic forcing. Rollbacks are tested only in disposable databases. sqlc reads migrations plus a generation-only schema for golang-migrate's `schema_migrations` table.

## Project layout

```text
backend/
  cmd/api/                composition and graceful shutdown
  cmd/seed/               development-only account and guide provisioning
  internal/auth/          sessions, bcrypt, CSRF, RBAC and login limiting
  internal/users/         account management and profile
  internal/departments/   department management
  internal/knowledge/     article/category/search and self-service operations
  internal/media/         private streamed file upload/range delivery
  internal/tickets/       ownership, comments, assignment, transitions and draft conversion
  internal/analytics/     authorized PostgreSQL reporting snapshots
  internal/audit/         administrator immutable audit browsing and metadata scrubbing
  internal/settings/      safe clinic preferences, versions and transactional audits
  internal/seed/          atomic synthetic account and guide seed
  internal/config/        validated environment configuration
  internal/middleware/    request safety and structured logging
  internal/platform/      HTTP parsing/responses and PostgreSQL/sqlc
  db/migrations/          versioned PostgreSQL schema
  db/queries/             sqlc query sources
  db/seeds/               synthetic reference data
  api/openapi/            implemented API specification
  tests/                  repository/HTTP integration checks
frontend/                 localized React/Vite application
infra/nginx/              same-origin reverse proxy
scripts/                  environment, integration and smoke tooling
docs/                     architecture, requirements, schema and phase reports
```

Read [architecture](docs/architecture.md), [requirements](docs/requirements.md), [database design](docs/database.md), and the [implementation plan](docs/implementation-plan.md).

All development data is synthetic. Do not enter patient records, private keys, credentials, or sensitive certificates into MedDesk.

## After the MVP

The five planned application phases are implemented. Before clinic rollout, complete the concrete deployment, privilege, retention/scanning and backup/recovery prerequisites in [operations](docs/operations.md). Local verification does not imply production deployment or browser visual acceptance.
