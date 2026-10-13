# Phase 3 delivery report — Knowledge Base and private media

Completed on 2026-10-09. Phases 1–3 are implemented and verified; Phases 4–5 remain pending.

## Delivered behavior

Employees can search published guides, read symptoms and ordered solution steps, view authenticated screenshots/videos, and answer “Бұл нұсқаулық көмектесті ме?”. Helpful feedback records a successful self-service attempt without creating a ticket. Engineers and administrators can create/edit articles, manage tags/media, preview drafts, publish/unpublish, and delete eligible articles. Administrators can manage localized categories. The Kazakh/Russian interface uses persisted PostgreSQL data, with loading/empty/error states, pagination, confirmations and notifications.

Search combines weighted PostgreSQL `simple`/`russian` full-text vectors with tag, literal substring and trigram word-similarity fallback. Kazakh stemming is not assumed. Employees cannot read drafts or inactive-category articles, including through direct UUID/slug lookup. Staff may preview hidden content. Publication requires an active category; existing content can be retained as a draft or unpublished after category deactivation.

Article updates/deletes require the editing version and reject concurrent changes with 409. Content, tags, media links and scrubbed audits commit atomically after fresh actor checks. Ticket-linked articles cannot be deleted. Views deduplicate per user/article for 15 minutes. Feedback uses an immutable attempt UUID: identical retries are no-ops, and changing the answer for that attempt returns 409. Metrics require published active-category content even for staff.

Staff can upload exactly one multipart PNG/JPEG image or MP4/WebM video. Images have a 10 MiB limit plus dimension/decode checks; videos default to 100 MiB with bounded container/video-track validation. Bytes stream to private persistent storage under generated keys. New article associations require matching purpose and uploader ownership or administrator authority; staff can retain media already attached to the article. Uploaders and administrators retain file access independently of publication. Other engineers require an article association; other employees require published active-category content. Authorization precedes GET/HEAD, conditional and byte-range processing.

Normal JSON deadlines remain bounded. Upload/download handlers have separate connection/context deadlines and bypass JSON timeout buffering. Nginx forwards media without request/response buffering, and the API enforces authoritative size limits. No public static media directory exists.

Migration 2 adds article versions and a view-deduplication index. Current readiness requires clean schema version 2. The development-only account seed now also provisions three safe synthetic NCALayer/printer/network guides. Repeating it preserves existing account/article state and attaches seed tags only to newly inserted articles. Private `.env` credentials were preserved.

## Changed and added files

- `backend/internal/knowledge/{model,service,repository,handler}.go`, `service_test.go`, `handler_test.go`: article/category/tag operations, search, visibility, versions, metrics and validation.
- `backend/internal/media/{model,service,repository,handler,files,validation}.go`, `service_test.go`, `handler_test.go`: private streamed uploads, validated containers/images, anchored filesystem operations, authorization and range delivery.
- `backend/db/migrations/000002_knowledge_version.{up,down}.sql`; `backend/db/queries/knowledge.sql`, `media.sql`; generated `backend/internal/platform/db/{knowledge.sql,media.sql,models,querier}.go`: schema/query contracts and typed persistence.
- `backend/tests/knowledge_integration_test.go`, `media_integration_test.go`, `knowledge_media_http_integration_test.go`: real PostgreSQL search/CRUD/audit/authorization and composed cookie-authenticated publication/media workflows.
- `backend/tests/auth_integration_test.go`, `schema_integration_test.go`: larger test-fixture budgets for race-enabled bcrypt work and schema version 2 expectations; production deadlines unchanged.
- `backend/db/seeds/knowledge.sql`, `development.sql`, `embed.go`; `backend/internal/seed/seed.go`: development-only, idempotent synthetic guide provisioning in the account seed transaction.
- `backend/internal/config/config.go`, `config_test.go`; `backend/internal/middleware/middleware.go`; `backend/internal/platform/health/{handler,handler_test,stream_test}.go`: validated media settings, non-buffered streaming deadlines, route dispatch and readiness metadata.
- `backend/cmd/api/main.go`, `backend/Dockerfile`, `backend/api/openapi/openapi.json`: module injection, version 0.3.0 and actual implemented API/authorization/media contracts.
- `frontend/src/pages/{Knowledge,Article,ArticleEditor,Categories,Overview}.tsx`; `frontend/src/App.tsx`, `components/Shell.tsx`, `components/KnowledgeState.tsx`: employee/staff/category routes and navigation.
- `frontend/src/lib/api.ts`, `knowledge.ts`, `phase3-locale.ts`, `useCategories.ts`, `locale.tsx`; `frontend/src/styles.css`: typed knowledge/media requests, bounded forms, localized messages and responsive presentation.
- `frontend/src/test/Knowledge.test.tsx`, `api.test.ts`, `App.test.tsx`; `frontend/package.json`, `package-lock.json`: knowledge/editor/API regressions and version 0.3.0.
- `.env.example`, `docker-compose.yml`, `infra/nginx/default.conf`, `scripts/smoke-auth.py`: media settings, authenticated streaming proxy/CSP and deployed knowledge smoke checks.
- `README.md`, `docs/architecture.md`, `requirements.md`, `database.md`, `implementation-plan.md`, this report: implemented scope, operational decisions, verification and next phase.

The existing CI workflow runs the extended checks. No remote GitHub Actions execution is claimed. This inventory describes Phase 3 work relative to the preceding phase; the repository has no baseline commit yet.

## Executed verification

- `go test -race ./...` and `go vet ./...` in `backend`: passed. Database-only tests skip without `TEST_DATABASE_URL`; their actual execution is covered separately below.
- `make test-frontend`: clean dependency install, TypeScript checks, **56 tests**, and production build passed. Vite reported a nonblocking 511.36 kB initial JavaScript chunk warning (157.25 kB gzip).
- Targeted knowledge/media race tests and four knowledge tests against disposable PostgreSQL 18 passed. They exercise Kazakh/Russian/tag-only relevance, hidden content, versions, media ownership/purpose, category filtering, immutable feedback and audit rollback.
- Complete `make test-integration`: **both full repeats passed** after the fixture-budget correction (206.838s and 207.504s). Reference-seed idempotency, migration down/up and removal of all application tables during rollback were verified. The run used its own PostgreSQL container, never the application's database/volumes.
- `make generate`: regenerated pinned sqlc output and verified matching before/after hashes. OpenAPI JSON and all local references validated.
- `make compose-check`, Bash syntax, Python smoke syntax and `.env` permission checks passed. `.env` remains private with mode 600.
- `make up`: built the final backend/frontend images, applied migration 2 and started healthy PostgreSQL/backend/frontend/Nginx. Nginx syntax validation and reload passed after mounted configuration changes. Public smoke returned 200 for all six API/SPA probes; backend reports version 0.3.0 and phase 3.
- Development seed executed twice; existing users were preserved, and fingerprints of the three guides' content, versions, publication and tags matched before/after the repeated seed.
- `make smoke-auth`: all three synthetic roles passed session/RBAC/CSRF/logout checks plus knowledge visibility, category/tag/detail access and multilingual searches. Anonymous knowledge access and missing-CSRF writes were rejected. Secrets were not printed.
- Headless Chrome against the final Docker application: actual category creation, image upload, draft creation/publication, employee search/detail/authorized image/helpful feedback, Russian localization, mobile layout and article deletion confirmation passed without runtime/CSP errors. Representative article, editor and mobile screenshots were visually inspected. This is a local developer check, not a committed browser CI suite; actual video codec playback was not claimed.
- Live Nginx/API upload of a synthetic **3,147,775-byte PNG** exceeded the previous 2 MiB gateway cap and returned 201. Authorized GET/HEAD/Range passed, including byte/hash equality, content length, 206 headers, `nosniff` and `private, no-store`. Anonymous GET returned 401; an SVG submitted as PNG returned 415. Browser/gateway fixtures were removed by exact IDs/storage keys, and scripted sessions were logged out.

## Issues found and fixed

- A background article refresh could supply a new version to an older form, allowing stale fields to overwrite another edit. Editors now retain their original editing version and preserve input on conflict.
- An inactive-category draft could not be saved when no active categories remained; the editor now preserves and allows its existing category for a draft/unpublish operation.
- A temporary background fetch failure could unmount a dirty form. Retryable failures preserve edits; authorization/not-found boundaries still hide unavailable content.
- React StrictMode could reuse an aborted upload controller. Upload lifetime handling was corrected.
- HEAD media routes could fall through the JSON timeout wrapper because mux patterns distinguish GET and HEAD. Both explicitly select the non-buffered stream handler, with a real HTTP flush regression.
- An uncertain PostgreSQL COMMIT must not trigger deletion of a possibly committed file. Definite failures clean up bytes; ambiguous commits retain private files for reconciliation.
- Tag-only matches needed an explicit search fallback. Repeating the demo seed also needed to preserve tags removed by an editor; seed links now attach only to new articles.
- The first combined PostgreSQL run failed a pre-existing management test when its 15-second transaction fixture expired during race-enabled bcrypt work. Shared database/authentication test fixtures now allow one minute. Both corrected full repeats passed; production request limits were not increased.

## Verify locally

```bash
make up
make seed-accounts
make smoke smoke-auth
make test
make test-integration
```

Open [MedDesk](http://localhost:8080). Use the existing synthetic account emails and private `DEMO_*_PASSWORD` values in `.env`. As an employee, search for `NCALayer`, `ЭЦҚ` or `принтеры`, open a guide and submit feedback. As an engineer, use `/engineer/knowledge` to create a draft, add safe synthetic media, publish and edit it. As an administrator, manage `/admin/categories`. Verify a draft/inactive-category guide is hidden from employees, and try two simultaneous edits to confirm the stale version is rejected. Switch Kazakh/Russian and test a narrow viewport.

## Limits and next phase

Video validation checks containers/tracks, not codecs, antivirus or transcoding. The browser must support the supplied encoding. WebP uploads are not supported. The UI caps videos at 100 MiB; the backend can be configured within 1–500 MiB. Unused uploads/article deletion do not automatically delete bytes; retention, quotas, orphan reconciliation and coordinated storage/database recovery remain operational work before clinic deployment. HTTPS, managed secrets, separate database privileges and tested backups also remain deployment requirements.

**Phase 4 is next:** suggest relevant guides before ticket creation, preserve successful self-service without a ticket, and implement employee ticket ownership, engineer assignment, statuses/priorities, comments, private attachments and transactionally recorded transitions. Database-derived analytics and remaining administration pages are Phase 5. Phase 3 completion does not mean the entire MedDesk MVP is finished.
