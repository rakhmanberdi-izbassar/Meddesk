# MedDesk incremental implementation plan

## Repository inspection

The initial repository contained only `.idea/` GoLand metadata, no application source, no Git repository, and no existing project conventions. The implementation initialized a local Git repository without creating a remote or publishing code. Go/Node/npm/Docker Compose/golang-migrate tools were available; toolchain and dependency versions are pinned in the project files.

## Phase 1 — foundation (complete)

1. Created architecture, requirements, database, and implementation-plan documents before application code.
2. Initialized Go API: validated configuration, pgx pool, generated sqlc foundation queries, liveness/readiness, version metadata, structured responses/logging, request deadlines, security headers, graceful shutdown, and OpenAPI.
3. Created the normalized PostgreSQL schema with constraints/indexes and reference roles, a development reference seed, and disposable integration checks.
4. Initialized React/TypeScript/Vite, Tailwind, shadcn/ui primitives, Router, Query, Form/Zod dependencies, Fetch, icons, locale dictionaries, responsive shell, and honest operational status.
5. Configured non-root application containers, PostgreSQL, one-shot migration, private media storage, frontend static server, Nginx, environment example, Makefile, README, and CI.
6. Executed Go race/vet, schema up/down/up integration, sqlc regeneration, frontend checks/tests/build, Compose validation, and container HTTP smoke checks.

Completed and verified on 2026-10-09. See the [Phase 1 delivery report](phase1-report.md) for its historical file inventory and checks. Phase 1's unavailable-login state has been replaced by Phase 2 authentication.

## Phase 2 — identity and management (complete)

Implemented:

1. bcrypt cost 12 password hashing, normalized email, hashed opaque sessions, expiry, login session rotation, logout, and `/auth/me`.
2. Exact allowed Origins, session-bound CSRF cookie/header checks, HttpOnly cookies, secure production configuration, bounded login rate limiting, and trusted-proxy IP handling.
3. Server role guards, protected routes, own display-name profile updates, administrator user/role/activation/password management, and department management with real filters/pagination.
4. Transactional audit records and account session revocation; shared management advisory lock, current-actor rechecks, self/last-admin protection, and active-user/active-department policy.
5. Login/account race protection and consistent users-before-sessions logout lock ordering.
6. Responsive Kazakh/Russian login, authenticated overview, profile, user management, and department management. Forms, loading/empty/error states, toasts, sensitive-change confirmations, dialog focus restoration, and identity-aware cache/form handling are implemented.
7. An explicit development-only Go demo seed with external password variables, atomic reference/account creation, and preservation of existing account state. Production/test mode rejects provisioning.
8. sqlc query sources/generated code, Phase 2 API documentation, configuration/infra updates, and tests. **Phase 2 added no schema migration; its readiness gate expected clean version 1. Phase 3 advances the current gate to version 2.**

### Executed phase verification

Go business/handler tests and vet, sqlc generation, frontend type checking/component tests/build, and isolated PostgreSQL integration checks were run. The final PostgreSQL race-enabled suite passed both before and after migration down/up, including five authentication repository tests and the real HTTP administration workflow. Final frontend tests passed with 36 cases. See the [Phase 2 delivery report](phase2-report.md) for files, fixes, checks and verification limits.

Checks cover actual bcrypt persistence, normalized/unique accounts, filters, role isolation, profile constraints, active-department policy, audit-failure rollback, session expiry/rotation/revocation, concurrent account-login behavior, and logout lock ordering. Frontend regressions cover session/cache identity changes, late profile responses, and dialog focus behavior. These Phase 2 checks did not cover knowledge/ticket browser scenarios.

### Reproducible verification

```bash
make up
make demo-env
make seed-accounts
make smoke
make smoke-auth
make test
make test-integration
make generate
make compose-check
```

Retrieve local demo passwords from the private environment configuration without copying them into tracked files or logs. In the running interface, sign in as each synthetic role; verify employee/engineer profile access and administrator-only user/department pages. As administrator, create/edit a user and department, try duplicate names/emails, attempt to deactivate an occupied department, and verify rejected operations preserve stored state. Reset/deactivate another account and confirm its previous session loses access. Confirm own demotion/deactivation is rejected and profile editing cannot change role/department. Switch Kazakh/Russian and exercise keyboard dialog close/focus behavior.

`make test-integration` creates and removes its own database; its down/up migrations never target the application's database. `make smoke` verifies the running deployment, not all feature flows. `make generate` should leave generated sqlc files consistent with committed queries; review differences before delivery.

Phase gate met: durable authenticated account/department flows work through the API and localized frontend, while unauthorized operations, invalid state changes, and unsafe authentication requests are rejected.

## Phase 3 — knowledge and media (complete and verified)

Implemented scope:

1. [Migration 2](../backend/db/migrations/000002_knowledge_version.up.sql) adds article optimistic versions and a per-user/article/date view index. Readiness now expects a clean version 2. Existing Phase 2 data is preserved by this additive migration.
2. [Knowledge handlers/services/repositories](../backend/internal/knowledge/) and [sqlc queries](../backend/db/queries/knowledge.sql) implement articles, administrator categories, tags, published/draft visibility, ranked multilingual search, versioned changes, media associations and transactional audits. Lists fetch ranked IDs then one detail batch. Search combines simple/Russian full-text vectors, tag/substring matching and title word similarity without assuming Kazakh stemming.
3. View POSTs deduplicate within 15 minutes; helpful/not-helpful attempt POSTs are immutable/idempotent. Both require published active-category content even for staff metrics. Hidden drafts do not contribute public view/feedback events.
4. [Media handlers/storage](../backend/internal/media/) provide staff-only single-file multipart uploads, PNG/JPEG image validation (10 MiB, bounded full decoding), structural MP4/WebM validation (100 MiB default), fresh-role transactional metadata/audit and private GET/HEAD/byte ranges. Files use an anchored filesystem root, generated keys and disk streaming; authentication/CSRF precedes upload bytes.
5. [Localized frontend routes](../frontend/src/App.tsx) implement knowledge/search/details, staff management/create/edit and administrator categories. Forms use actual APIs, ordered steps, tags/media/publication, confirmations and error/empty states. The editor preserves its initial version/unsaved input during background refresh and rejects stale writes.
6. Separate JSON/upload/download deadlines and [Nginx streaming configuration](../infra/nginx/default.conf) prevent buffering entire video responses. Configuration validates upload/stream timeouts and the video byte limit. Backend video limits can rise to 500 MiB; the frontend remains capped at 100 MiB.
7. [Development knowledge seed](../backend/db/seeds/knowledge.sql) adds three synthetic NCALayer/printer/network guides within the existing development-gated account seed transaction, preserving existing accounts/articles and attaching seed tags only to new articles. No production seed or private signing/patient material is introduced.

Authorization and lifecycle decisions:

- Any authenticated role reads published content in active categories; staff preview/manage drafts or inactive-category content. Category changes are administrator-only. Publishing requires an active category; drafts/unpublishing remain editable after deactivation.
- Article update/delete compares the current version, and ticket-linked articles cannot be deleted. New media requires proper purpose and uploader ownership or administrator authority; another engineer may retain already-attached media on the same article.
- An unattached upload is readable by its uploader/administrator. Article-attached media is also readable by engineers, and published active-category media is readable by authenticated employees. Access checks precede conditional/range responses.
- WebP is not accepted by the upload API. Container checks do not promise codec playback, scanning or transcoding. Known upload failures remove bytes, while an ambiguous PostgreSQL COMMIT retains the private file for reconciliation. Crashes may leave orphans. Article deletion/replacement does not delete unused media; automatic cleanup, retention, quotas and backup/recovery policy remain operational follow-up work.

### Executed implementation checkpoints

Seven knowledge service/handler tests passed under `go test -race ./internal/knowledge`, and `go vet ./internal/knowledge` passed. Four knowledge repository integration tests passed under the race detector against disposable PostgreSQL 18 with migrations 1+2 (latest targeted run: 1.941s). They verify actual Kazakh/Russian/tag-only search, publication/category visibility, localized category management, versions, view/feedback retries, fresh authorization, media ownership/purpose and audit rollback.

The final frontend type/component/build suite passed with 56 tests, including editor concurrency, unsaved-input and upload-lifecycle regressions. Complete Go race/vet checks passed. Both full PostgreSQL integration repeats passed, before and after migration down/up, including the composed HTTP knowledge/media workflow. See the [Phase 3 delivery report](phase3-report.md) for all changed files, fixes, commands and verification limits.

### Completed phase gate

1. Complete Go race/vet and frontend type/component/build suites passed after the fixes.
2. sqlc regeneration hashes matched; Compose and OpenAPI validation passed.
3. The disposable PostgreSQL suite passed twice through migration up, reference-seed checks, down/up and composed HTTP/media flows.
4. Docker was rebuilt and is healthy; repeated development seeds preserved content; deployed role/search/upload/private GET/HEAD/range checks passed, including a 3,147,775-byte PNG through Nginx.
5. Knowledge/editor/category browser flows, localized rendering and desktop/mobile layouts passed; representative screenshots were visually inspected and the Phase 3 report was completed.

Use the reproducible commands documented in [README](../README.md); local development credentials remain in ignored private environment configuration. `make test-integration` owns its disposable database and never rolls back application data. Phase 3 acceptance is met; operational media retention/scanning/recovery limitations remain documented before clinic deployment.

Phase 3 boundary: ticket creation/management was deferred to Phase 4. The current application now implements it as described below.

## Phase 4 — ticket workflow (complete and verified)

Implemented published guide suggestions and immutable feedback before ticket creation; persistent ticket lists/details, public/internal comments, owned screenshot attachments, active staff assignment, priority changes and validated status transitions. Current requester/department are derived from PostgreSQL identity. Every mutation compares the ticket version and rechecks actor authority inside the shared-lock transaction; history, resolution timestamps, associations and scrubbed audits commit together. Requesters can confirm resolved-ticket closure; only administrators explicitly reopen closed tickets.

Migration 3 adds one reviewed draft-conversion reference per ticket and advances readiness to clean version 3. Engineers/administrators can convert resolved/closed tickets to a single unpublished guide; text is reviewed and screenshots are not copied automatically. Versioned Kazakh/Russian `/tickets`, `/tickets/new` and `/tickets/:id` pages implement lists/filters, suggestion feedback, screenshots, discussions/history, assignment/transitions, requester confirmation and article-editor conversion. Conflicts preserve unsaved input until an explicit refresh confirmation.

Completed on 2026-10-12. Go race/vet, all 76 frontend tests/type checks/build, two full disposable PostgreSQL integration passes through migrations up/down/up, sqlc consistency, Compose/OpenAPI checks, Docker health and role-aware live smoke passed. Browser automation was unavailable; visual browser acceptance remains unexecuted. Verification and limitations are recorded in the [Phase 4 delivery report](phase4-report.md). Analytics, audit browsing and settings remain Phase 5.

## Phase 5 — analytics and completion (complete and verified)

Implemented database-derived reporting with bounded local-date/timezone/department/category/assignee filters, explicit current-cohort/reopened-ticket semantics, independent feedback/view aggregates, daily series and historical catalogs. Administrator audit browsing sanitizes typed metadata from stored events. Versioned clinic settings update atomically with audit, control reporting defaults and new-ticket priority, and preserve user form choices.

Role dashboards, staff analytics and administrator audit/settings pages are localized and use actual APIs. Migration 4 preserves existing data and advances readiness to version 4. Operations documentation and a coordinated development backup plus disposable restore check cover recovery and concrete production prerequisites. See the [Phase 5 delivery report](phase5-report.md) for executed checks and remaining operational/browser limits.

Completed on 2026-10-12. Backend race/vet, 94 frontend tests/type/build, two full disposable PostgreSQL integration runs with migrations 1–4 down/up, sqlc/Compose/OpenAPI checks, independent backup/restore, deployed role smoke and seed preservation checks passed. Browser visual acceptance was unavailable.

The five planned application phases are implemented. Clinic production rollout requires the separate operational prerequisites; no next application phase is implicitly started.

## Definition of done for every phase

- Features are real and match documented authorization/persistence semantics.
- Relevant tests and builds are executed; failures are fixed or transparently reported.
- Changed files, verification commands, current limitations, and next phase are documented.
- No production credentials, patient data, fabricated statistics, or incomplete feature claims are introduced.
- Stop at the requested phase boundary; begin the next phase when requested.

## Private school workspace delivery

Implemented a private owner-only school module, separate from clinic records and reporting: device inventory (including network and surveillance equipment), device name/inventory search, linked incident history, requests, status transitions, version checks, private transactional history and bilingual navigation. Migration 6 grants no automatic owner. Clinic account administration cannot reset or edit another account that owns the private workspace.

Verification uses backend unit/handler tests, the school PostgreSQL integration scenario, the full migration/integration script, frontend component tests and the production frontend build. Activation requires migration 6, deployment and explicit assignment of the existing owner account by a trusted database operator. See `school-workspace.md` for commands and supported boundaries.

Migration 7 now delivers school employee accounts and self-submission; school knowledge articles, attachments and camera monitoring remain future work.
