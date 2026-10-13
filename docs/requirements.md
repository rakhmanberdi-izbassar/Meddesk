# MedDesk requirements

## Objective and current delivery

Reduce repetitive clinic IT calls through self-service guides, educational videos, and a support ticket workflow. The complete MVP must provide durable data, server-side validation/authorization, and reproducible tests. Do not process patient medical information.

**Phases 1–5 are implemented.** Available code provides identity/administration, knowledge CRUD/search, private media, self-service feedback, ticket workflows, real staff analytics, administrator audit browsing and persisted clinic settings. See the Phase 5 delivery report for verification and limits.

## Roles

- Employee: signs in, views the authenticated overview/system status, updates their own display name, searches/reads published guides in active categories, watches authorized media and records feedback. Creates tickets after guide suggestions, uploads screenshots, tracks own tickets, adds public comments and confirms resolved-ticket closure.
- IT engineer: has authenticated overview/profile access, knowledge drafts/publication/versioned editing/deletion and approved image/video uploads; cannot use administrator user/department/category mutation APIs. Manages ticket assignment/priorities/transitions, internal comments, screenshots and reviewed draft conversion. Uses real operational analytics and its filters.
- Administrator: manages users, roles, account activation/password resets, departments and knowledge categories; can also manage articles/media and their own profile. Can manage all support tickets and explicitly reopen closed tickets. Browses immutable sanitized audit events and edits versioned clinic settings.

Role-sensitive navigation is an interface convenience; every protected API enforces authentication and role permissions independently.

## Phase 2 acceptance criteria (implemented)

- Login/logout/current-session endpoints use opaque server-side sessions and bcrypt cost 12 passwords. Password length is 12–72 UTF-8 bytes; normalized email is case-insensitively unique.
- Production cookies are HttpOnly/Secure/SameSite=Lax with bounded lifetime. Login validates the exact configured Origin. Authenticated mutations require that Origin and a matching session-bound CSRF cookie/header.
- Login is rate-limited by client IP and normalized account; forwarded IP headers are used only for configured trusted peers. Limits are bounded, process-local, and reset when the API restarts.
- Sessions expire, rotate on successful login, and are revoked on logout. Current database role/active status is checked for each protected request. Password/role/email/activation changes revoke all account sessions atomically, including when reactivating an account.
- Administrators list/create/update users with search, role, active filters and pagination. User password hashes are never returned. There is no self-registration or account deletion endpoint.
- Administrators list/create/update departments with search, active filters and pagination. Names are case-insensitively unique; deactivation preserves references.
- Active users require an active department. Department deactivation is rejected while active users remain. User/dept mutations serialize consistently and recheck the acting administrator's current authority within the transaction.
- Administrators cannot demote/deactivate themselves or remove the last active administrator. Management changes and scrubbed audit events commit together; failures roll back the mutation.
- Any active authenticated role can change only their own display name through `/profile`; email/role/department/password/activation cannot be changed through that endpoint.
- All request bodies use bounded strict JSON, supported-field validation, and parameterized SQL. List filters/UUIDs are validated server-side; duplicate records and policy violations return structured errors.
- The localized responsive interface implements `/login`, `/`, `/profile`, `/admin/users`, and `/admin/departments`, including real CRUD tables, loading/error/empty states, keyboard-accessible forms/dialogs, toasts, and confirmations.
- Private frontend caches clear when authentication is lost or account/role changes; a late response from a different account cannot overwrite current identity. Form state remounts on identity changes.
- Demo provisioning runs only for explicit `APP_ENV=development`, uses externally supplied random passwords, and preserves existing accounts. It is not automatic production startup behavior.

## Phase 3 acceptance criteria (complete and verified)

- Articles persist a unique slug, title, description, category, symptoms, ordered plain-text solution steps, optional video/ordered screenshots, tags, author, publication timestamps/status, created/updated timestamps and optimistic version. DTO counters come from actual view/feedback rows.
- `/api/v1/knowledge` exposes article lists/details/CRUD, categories, tags, views and feedback. Default article search includes published active-category guides; employees cannot request drafts/all or retrieve hidden article details/tags. Staff can explicitly inspect/manage drafts and inactive-category content.
- Engineers/administrators create, update, publish, unpublish and delete articles. Category mutations require administrator authority. Deactivation hides content without deleting history. Publishing requires an active category; retaining drafts or unpublishing under an inactive category remains allowed.
- Article updates/deletes require the current version; concurrent changes return `409 version_conflict`. The editor retains unsaved input and its initial editing version, including during background query refresh. Ticket-linked articles cannot be deleted; unpublishing is the available alternative.
- Content, tags/images and scrubbed audit metadata commit together. Mutation authority is rechecked in PostgreSQL after taking the shared administration/knowledge/media transaction lock. An audit failure rolls back the mutation.
- Search ranks title/full-text relevance using PostgreSQL `simple` and `russian` vectors, plus case-insensitive tag/substring and trigram word-similarity fallback. No Kazakh stemmer is assumed. Server-side filters and pagination are bounded/parameterized. Search does not create a ticket; Phase 4 uses published search for pre-ticket suggestions.
- Views and feedback are explicit session-CSRF-protected POSTs for published articles in active categories. Views deduplicate per user/article for 15 minutes. Feedback uses a user/article/attempt UUID unique key: same-answer retries are no-ops, conflicting answers return 409, independent attempts remain possible. Helpful feedback records the self-service event without creating a support ticket.
- Staff upload one multipart file with an `ARTICLE_IMAGE` or `ARTICLE_VIDEO` purpose. Employees cannot upload article media. Files use random filesystem keys, never user filenames or database video bytes. Names, streamed file/body sizes, type signatures and purpose are validated server-side.
- Uploads allow PNG/JPEG up to 10 MiB and MP4/WebM up to 100 MiB by default. The backend video limit is configurable from 1 to 500 MiB, while the frontend caps videos at 100 MiB. WebP remains unsupported by the current API despite its database allowance. Images are fully decoded after enforcing 10,000-pixel axis/40-million-pixel bounds. Video checks validate container structure/video tracks, not codecs, antivirus or transcoding.
- New article media associations require matching purpose and uploader ownership or administrator authority; another engineer may retain existing media on the same article. Unattached uploads are private to their uploader/administrator. Other engineers may read article-attached media, and any authenticated role may read published active-category media.
- Media GET/HEAD/byte-range delivery rechecks access before conditional responses, uses no-store/private headers and safe anchored filesystem opens, and exposes no storage key/hash in JSON. Nginx streams through the authenticated API without a static media alias. JSON and streaming handlers have separate bounded deadlines.
- Known upload failures remove bytes; ambiguous PostgreSQL commit outcomes retain private files for operator reconciliation. Crash orphans, unused uploads, retention/cleanup, quotas and coordinated storage/database recovery remain operational follow-up work. Article deletion/replacement does not delete media bytes automatically.
- Kazakh/Russian routes include employee `/knowledge`, `/search`, `/knowledge/articles/:id`, staff `/engineer/knowledge` and create/edit pages, and administrator `/admin/categories`. They use actual APIs with responsive layouts, loading/error/empty states, accessible forms, toasts, publication controls and deletion confirmation.
- Development provisioning adds three safe NCALayer/printer/network guides in the existing development-only transaction, preserving existing accounts and article content/publication/counters. Examples contain no signing keys, passwords, real certificates or patient records.

## Ticket workflow (Phase 4 implemented)

1. Employee describes the problem; the API searches published articles.
2. Relevant articles appear with “Бұл нұсқаулық көмектесті ме?”.
3. Helpful feedback records successful self-service and ends without creating a ticket.
4. If unresolved, create a ticket with title, description, authenticated employee/department, priority, category, related articles, and attachments.
5. Engineers use `NEW`, `IN_PROGRESS`, `WAITING_FOR_USER`, `RESOLVED`, and `CLOSED`; priorities are `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`.

Enforced transition policy: `NEW -> IN_PROGRESS`; `IN_PROGRESS -> WAITING_FOR_USER|RESOLVED`; `WAITING_FOR_USER -> IN_PROGRESS|RESOLVED`; `RESOLVED -> IN_PROGRESS|CLOSED`. Closed tickets are immutable unless an administrator explicitly reopens them to `IN_PROGRESS`. Resolution timestamps, history, actor, and audit changes must commit atomically. Employees cannot arbitrarily change status, assignment, or ownership; a dedicated confirmation operation closes their own resolved ticket. Concurrent changes must not silently overwrite each other.

The implemented ticket API adds versioned public/internal comments and owned screenshot attachments, staff assignment/priority changes, requester closure confirmation and explicit administrator reopening. Resolved/closed tickets can create one reviewed unpublished draft guide in the same audited transaction. Mutation authority is rechecked against current database roles; stale versions return a conflict and preserve frontend input. Up to ten screenshots and ten published related guides are supported.

## Analytics and administration (Phase 5 implemented)

Statistics come from PostgreSQL: totals/open/resolved counts, average resolution time, department/category breakdowns, common issues, popular articles, and self-service rate. Define self-service rate as helpful troubleshooting attempts divided by all completed helpful/not-helpful troubleshooting attempts in the selected period; views alone do not count. Document timezone and reopened-ticket semantics in Phase 5. Empty data must produce honest empty states.

Staff reports, administrator audit/settings pages and role dashboards are implemented. Reporting uses a consistent PostgreSQL snapshot, inclusive local dates, current-state creation cohorts and absent measurements for empty averages/feedback ratios. Audit metadata is action-allowlisted and type-checked on reads, even for legacy records. Settings updates require current administrator authority and an optimistic version, commit with audit, and never expose deployment secrets. The composed synthetic self-service/ticket/guide/reporting workflow is tested for all roles. See README for exact timezone/reopened-ticket/filter semantics and operations documentation for recovery/prerequisites.

## Interface

Kazakh is the default; Russian localization is selectable and persisted. Require a responsive medical/enterprise appearance, sidebar/top navigation, accessible labels and keyboard interaction, filtered/paginated lists, loading/error/empty states, toast notifications, and mutation confirmations. Phases 2–5 implement these patterns for identity, administration, knowledge, tickets, analytics, audit and settings.

## Security acceptance criteria

Implemented identity protection includes password hashing, opaque hashed sessions, session-bound CSRF, exact Origin validation, secure production cookies/configuration, authentication rate limiting, role guards, bounded input, parameterized queries, session revocation, and transactional account audits. Do not log request bodies, passwords/hashes, cookies/tokens, connection strings, or patient data. Do not use wildcard credentialed CORS.

Phase 3 implements media ownership/access, safe upload type/size handling, anchored path traversal protection and versioned knowledge mutations. Combined Phase 3 integration/browser verification passed. Phase 4 adds ticket ownership, internal-comment isolation and ticket attachment permissions. Clinic deployment additionally requires HTTPS ingress, managed secrets, appropriate database privileges, backups, and account lifecycle policy.

## Testing and delivery

Every phase requires executed checks, fixes for discovered issues, changed-file reporting, verification commands, and an explicit next phase. Do not claim a check passed unless it was executed.

Phase 2 verification covers service validation/security policies, protected handlers, session expiry/rotation/revocation, origin/CSRF/rate limiting, actual PostgreSQL CRUD/filtering/audit rollback, lock ordering and login/account races, and a real HTTP-to-PostgreSQL identity/administration workflow. Frontend component/API tests cover sign-in/out, role routes, forms/mutations, cache clearing, localization, sanitized errors, and accessible dialogs. This remains a verified Phase 2 workflow; it does not by itself verify the complete knowledge/ticket MVP.

Phase 3 executed checks passed: complete Go race/vet, frontend type/build and 56 tests, two full PostgreSQL 18 integration repeats through migration up/down/up, sqlc regeneration consistency, Compose/OpenAPI validation, live role/knowledge/media HTTP checks and Kazakh/Russian browser flows. They cover multilingual/tag-only search, hidden content, category management, media associations, versions, event retries, audit rollback and private range delivery. The [delivery report](phase3-report.md) lists changed files, discovered/fixed issues, reproducible commands and verification limits. This phase does not yet verify the planned ticket/analytics MVP.

Phase 4 checks passed: backend race/vet, 76 frontend tests/type checks/build, two full disposable PostgreSQL suites through migration up/down/up, sqlc consistency, Compose/OpenAPI validation and deployed role-aware smoke. Browser visual acceptance was unavailable in this session; see the [Phase 4 delivery report](phase4-report.md).

Phase 5 checks passed: backend race/vet, 94 frontend tests/type/build, exact reporting and complete multi-role SQL/HTTP workflows in two full disposable PostgreSQL suites, migration 1–4 up/down/up, sqlc/Compose/OpenAPI validation, isolated backup/restore and deployed role-aware smoke/seed preservation. Browser visual acceptance was unavailable; see the [Phase 5 delivery report](phase5-report.md).

Deliverables include README, environment example, Compose, Makefile, migrations, safe seed commands, OpenAPI, design documents, and CI. `make test` runs Go race/vet and frontend type checks/tests/build; `make test-integration` uses a disposable PostgreSQL database, repeats the suite after migration down/up, and never destroys project data. `make generate`, `make compose-check`, and `make smoke` verify query generation, infrastructure configuration, and the running gateway/API.
