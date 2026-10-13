# Phase 5 — analytics and MVP completion

Completed on 2026-10-12. API/frontend/Docker version is `0.5.0`; readiness requires clean migration version 4. The five planned application phases are implemented. Clinic production deployment remains subject to the concrete prerequisites in [operations](operations.md).

## Delivered features and semantics

Staff analytics uses real PostgreSQL data with inclusive local date/timezone and department/category/assignee filters, bounded to 366 calendar days. Reports and filter catalogs recheck current role authority and use read-only repeatable-read snapshots. Daily series include empty days and an accessible table alternative. Inactive historical filter records remain available.

Ticket counts describe tickets created in the interval and their current status. Resolved includes closed; closed is a subset. Average resolution spans original creation to current resolution for currently resolved/closed tickets. Reopened open tickets are excluded, and later resolution starts from original creation. Daily resolved bars are creation-day cohort subsets, not resolution events. Common issues rank category buckets, including uncategorized tickets.

Self-service rate is helpful/all completed immutable feedback attempts in the interval; an empty denominator is no measurement. Views alone do not imply self-service. Separate view/feedback aggregates prevent cross-join inflation. Assignee filtering affects tickets only. Feedback/views use the user’s current department and the article’s current category because earlier events have no historical membership snapshot; this is explicit in the UI/README.

Administrators browse read-only audit logs with local date, actor, action, entity and bounded pagination filters. Stored metadata is action-allowlisted and type-checked before returning it, even for legacy records; arbitrary text, nested secrets and malformed allowed-field values are removed. Nullable/system actors remain representable. No audit mutation endpoint exists.

Authenticated users read safe application settings. Administrator writes compare optimistic versions after fresh-authority/shared-lock/row-lock checks and commit with audit. Settings include clinic name, initial ticket priority and reporting timezone; deployment credentials/cookie/origin policies remain separate. Ticket API creation with omitted priority reads the current default inside its transaction. The form captures initial settings without overwriting a user choice and waits for that initial default unless the user chooses a priority or an explicit unavailable-settings fallback applies. Seeding preserves existing accounts, articles and settings.

Role dashboards display real own-ticket/staff report totals. Navigation and Kazakh/Russian pages cover analytics, audit and settings. Form snapshots preserve unsaved settings input; stale writes require an explicit refresh confirmation. Account/role changes clear private cached data and invalidate late previous-identity responses.

Development backup tooling pauses only a running development API, captures PostgreSQL/private media together with restrictive permissions/checksums, and restarts it. A separate disposable restore exercise verifies actual recovered data, schema version, settings and media bytes. Operations documentation covers recovery, retention, privilege separation and assessed production prerequisites.

## Changed areas

- `backend/internal/analytics`, `audit`, `settings`; their SQL sources and generated query code; migration `000004_application_settings`.
- `backend/cmd/api/main.go`, `backend/Dockerfile`, readiness/system metadata and schema integration assertions.
- Ticket service/repository default-priority integration and regression checks; seed CLI preservation message/comment.
- `backend/tests/phase5_integration_test.go` and package policy/HTTP/sanitization tests.
- Frontend analytics/audit/settings pages, reporting controls/schemas, settings hook, localized labels, role overview/navigation, ticket priority snapshot and component regressions; package version 0.5.0.
- OpenAPI, expanded read-only role smoke, `backup-dev.sh`, `test-backup.sh`, Makefile, ignored backup path and operations/requirements/design/implementation documentation.

## Executed verification

- `make test-backend`: complete Go race tests and vet passed. Package tests cover role/Origin/CSRF enforcement, bounded filters/JSON, civil date windows/DST, typed metadata sanitization, fresh-role authorization, settings versions and audit rollback.
- Five focused Phase 5 real PostgreSQL tests passed under the race detector in 33.528s.
- `make test-integration`: both complete PostgreSQL 18 race-enabled suites passed (199.603s / 230.601s) before and after migrations 1–4 down/up, plus reference-seed idempotency. They include exact aggregate/null/filter values, civil-day boundaries, fresh role checks, malicious legacy metadata, settings concurrency and full helpful-without-ticket → unresolved ticket → engineer resolution → owner closure → draft guide → analytics/audit workflow.
- Final frontend dependency installation/typecheck, all **94 tests** and production build passed. Initial broad checks exposed cold lazy-page readiness waits under test/install contention; the shared fixture now uses a bounded three-second readiness budget, preserving component assertions. New regressions cover role data/cache removal, exact allowed filter URLs, empty measurements, settings confirmation/conflicts/late responses and priority choices.
- sqlc regeneration hashes matched; Compose and OpenAPI references/operation IDs validated.
- `make test-backup`: isolated migrated PostgreSQL/media backup, checksums, database restore, schema/settings/media content and original source-backend restart passed; only disposable services/volumes/files were removed.
- Docker rebuilt and all four application services are healthy. Public smoke, expanded three-role authentication/reporting/audit/settings permission smoke, deployed Phase 5/version0.5.0 metadata/OpenAPI and SPA deep links passed.
- Deployed development seed rerun preserved privately compared existing account credentials/roles/activation and the complete clinic settings row. No credentials or password hashes were printed.

## Remaining limits

Browser inventory was empty, so live visual/keyboard browser acceptance and screenshots were unavailable in this session. Component tests and real HTTP/PostgreSQL workflows passed; layouts were reviewed in source. Local results do not claim remote CI execution.

Standard Compose remains development HTTP/privileged PostgreSQL. Production HTTPS, managed secrets/certificates, restricted runtime versus migration privileges, initial administrator provisioning, backup policies and measured recovery objectives require deployment work. Existing media reconciliation/retention/quotas/codec/malware limitations remain documented. No production deployment, patient data or fabricated analytics was introduced.
