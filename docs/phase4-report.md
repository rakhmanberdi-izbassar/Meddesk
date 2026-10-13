# Phase 4 — support ticket delivery report

Completed and verified with the checks below on 2026-10-12. The current application is version `0.4.0`, with clean migration version 3. Phase 5 analytics, audit browsing and settings remain outside this milestone.

## Delivered behavior

The localized ticket creation form searches published guides before submission. Employees can read suggested steps and record immutable helpful/not-helpful attempts; helpful feedback completes self-service without creating a ticket. Unresolved issues continue to a bounded title/description, priority, optional active category, reviewed article references and up to ten PNG/JPEG screenshots. Requester and department are derived from current database identity.

Employees list/read only their own tickets and public comments. Engineers/administrators filter all tickets, assign active staff, change priorities, add public/internal comments and manage valid status transitions. Internal comments are excluded by SQL for employee reads. Screenshot uploads retain private storage, content verification, byte limits and authentication; another requester cannot download them, including through HEAD/range/conditional requests.

All ticket mutations compare the expected version after taking the shared management advisory lock, rechecking the current actor and locking the ticket. Comments and attachment additions also advance the version. History, resolution timestamps, associations and scrubbed audit metadata commit atomically. Closed tickets refuse ordinary updates, comments and attachments. Requesters can confirm resolved-ticket closure; administrators explicitly reopen closed tickets with a reason. Reopening clears `resolved_at`.

Staff can convert a resolved/closed ticket once to an unpublished guide with reviewed text. Draft creation, ticket linkage and both audits share one transaction. Migration 3 records `converted_article_id` with a restrictive unique reference. Conversion never publishes or automatically copies ticket screenshots.

The frontend keeps form values and their initial version together. A background query cannot replace unsaved ticket or conversion input. Conflicts require an explicit refresh confirmation before discarding edits. Kazakh is the default and Russian strings cover the same workflows. Mobile layouts extend the existing interface patterns.

## Changed areas

- `backend/internal/tickets/`: ticket models, validation/policy, SQL repository, guarded handlers, service and HTTP tests.
- `backend/db/queries/tickets.sql`, `media.sql`, generated `backend/internal/platform/db/` and migration `000003_ticket_conversion`: persistence, current-role reads and conversion provenance.
- `backend/internal/media/`: employee ticket screenshot uploads, existing image verification/cleanup, purpose and download authorization extensions.
- `backend/Dockerfile`, `backend/cmd/api/main.go` and `backend/internal/platform/health/`: ticket registration, version/phase metadata and schema version 3 readiness.
- `backend/tests/tickets_http_integration_test.go`: composed HTTP and transactional PostgreSQL scenarios; schema integration expects migration 3.
- `frontend/`: ticket list/create/detail, uploads, schemas, localization, routes/navigation, knowledge-to-ticket links and reviewed article-editor conversion.
- `backend/api/openapi/openapi.json`, `scripts/smoke-auth.py`, README and design/implementation documents: current API contracts, safe read-only ticket smoke checks and milestone documentation.

## Verification

Go race tests and vet passed for the backend. Targeted ticket tests against separately migrated disposable PostgreSQL passed under the race detector in 30.918 seconds. They cover owner/internal-comment isolation, attachment purposes/uploaders/private downloads, assignment checks, all lifecycle operations, same-version concurrent winner/conflict behavior, fresh role downgrade, single draft conversion, and audit/history/conversion failure rollback. Media tests also verify employee screenshots without granting article-upload authority.

The initial full integration run exposed a test-fixture login deadline under race-detector bcrypt overhead. Its test-only router/client budgets were increased to match sibling HTTP fixtures; production deadlines were unchanged. The corrected `make test-integration` passed both complete race-enabled PostgreSQL suites (193.020s before rollback, 159.780s after reapplying migrations 1–3). Reference-seed idempotency and complete migration up/down/up checks passed. The final frontend `make test-frontend` passed dependency installation, TypeScript checks, all **76 component/API tests** (including 20 new ticket tests), and the production build. A cold lazy-route wait in the first broad frontend run was corrected to await page loading explicitly. Reviews also fixed multiline suggestion queries, invisible references after another search, unchanged inactive-assignee revalidation, ambiguous helpful feedback retries, and conversion input loss on background ticket changes. Docker was rebuilt and all application containers are healthy. Migration 3 was applied to the existing development database. `make smoke` and the expanded `make smoke-auth` passed at the loopback gateway for employee, engineer and administrator sessions, including ticket/catalog visibility, published suggestions, CSRF/Origin checks and logout. Deployed `/system` and OpenAPI report Phase 4/version `0.4.0`; the `/tickets/new` SPA deep link serves successfully.

sqlc regeneration hashes matched. Compose validation and OpenAPI JSON/local references passed.

## Verification limits

The browser automation provider reported no available browser, and opening the in-app browser was unavailable. Live visual/keyboard browser acceptance and screenshots were therefore not executed in this session. Component tests and real HTTP/PostgreSQL checks cover the implemented behavior; responsive CSS was reviewed in source. Local results do not claim that remote CI has run.

The production frontend build reports a nonblocking 524.89 kB main-chunk size warning. Ticket list detail loading currently uses one query per returned ticket, with bounded pages of at most 100; a batched read is a future performance improvement. Existing media orphan reconciliation, retention/scanning, backups and production deployment prerequisites remain as documented in README. No analytics or audit browsing is simulated.
