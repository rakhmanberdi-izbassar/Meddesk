# MedDesk database design

## Conventions and implementation status

PostgreSQL is the source of truth. UUID primary keys use `gen_random_uuid()`; timestamps use `timestamptz` in UTC. Foreign keys explicitly define deletion behavior. Deactivation preserves historical records. Ticket numbers have an immutable identity sequence and can later be formatted as `MD-000001`. Schema changes use versioned golang-migrate up/down migrations; generated sqlc queries use `pgx/v5`.

Phases 1 and 2 used **migration 1**. Phase 3 adds **migration 2** for article optimistic versions and an article-view deduplication index. Phase 4 adds **migration 3** with a nullable, unique `tickets.converted_article_id` reference for one reviewed draft conversion per ticket. Phase 5 adds **migration 4** for singleton `application_settings` and a ticket creation-time analytics index. Current readiness requires a clean version 4. Existing data is preserved.

## Implemented identity and administration

- `roles`: unique fixed codes `EMPLOYEE`, `IT_ENGINEER`, `ADMIN`, inserted by migration 1.
- `departments`: UUID, case-insensitive unique trimmed name, active flag, created/updated timestamps. Names are limited to 150 characters; application validation also rejects control characters.
- `users`: UUID, unique case-insensitive email, display name, bcrypt hash, role FK, required department FK, active flag, timestamps. The API normalizes email, validates names, and hashes passwords at bcrypt cost 12. DTOs omit the hash.
- `sessions`: fixed 32-byte SHA-256 token and CSRF hashes, user FK, expiration, creation/last-seen timestamps, and consistency checks. Raw tokens/passwords are not persisted. Lookup joins the current user and role, rejects expired sessions and inactive users, and does not rely on a role snapshot in the cookie.
- `audit_logs`: actor FK, action, entity type/id, JSON-object metadata, timestamp. Authentication and management writes are transactional. No audit administration UI or query API exists yet.

Lists apply parameterized text/role/active filters, deterministic ordering, and bounded pagination. User search matches normalized email/display name; department search matches names. Indexes cover lowercased email/name uniqueness, department/role user references, and session owner/expiration.

### Transaction rules

User/department administration and knowledge/media writes acquire the same transaction advisory lock. Administration then verifies that the acting user is still an active administrator; knowledge/media services recheck their own current role requirements. Row locks protect target accounts/departments. The last active administrator cannot be removed, and administrators cannot demote or deactivate themselves.

Active users require an active department. Deactivating a department with active users returns a conflict; users must first be transferred or deactivated. These cross-table rules belong to services rather than database check constraints. The shared lock makes creation/reactivation and department deactivation serialize consistently.

User role, password, email, or activation changes delete all sessions for that user and append a scrubbed audit record in the same transaction. Reactivation also deletes any old session records. Profile updates change only the authenticated user's display name and write an audit record atomically. An audit insertion failure rolls the mutation back. Normal administration uses soft deactivation, not deletion.

Login locks the user and rechecks active state/password hash before creating the session, rotating a presented previous session, deleting expired owner sessions, and recording the event. Logout looks up the owner, locks that user, then deletes the session and records the event. Both use user-before-session lock ordering to avoid deadlocks with account administration.

## Implemented knowledge persistence (Phase 3)

- `knowledge_categories`: unique slug, Kazakh/Russian names, optional description, active flag.
- `knowledge_articles`: unique slug, title, description, category, symptoms, ordered JSON solution steps, author, optional video, publication flag, integer optimistic version (migration 2), timestamps. Constraints validate nonempty content/JSON shape and publication timestamp consistency.
- `knowledge_tags` and `article_tags`: normalized unique tags and composite article/tag key.
- `article_feedback`: article, user, helpful flag, attempt ID, timestamp. The user/article/attempt unique key supports deduplication while permitting later independent attempts.
- `article_views`: article, user, timestamp for aggregate and period reporting; migration 2 indexes `(article_id, user_id, created_at DESC)` for the 15-minute per-user repeat-view check.
- `article_media`: article/media composite relationship and ordered screenshot positions.

[knowledge.sql](../backend/db/queries/knowledge.sql) implements ranked full-text search with generated weighted `simple` and `russian` vectors, tag and case-insensitive substring matching, and title word similarity. `simple` preserves Kazakh tokens without assuming Kazakh stemming. GIN/full-text and trigram indexes provide the schema search foundation. The application constrains employee reads to published articles in active categories; staff may explicitly request drafts/all content. Tags exposed to employees must be attached to visible articles. Ranked IDs are fetched before one batch detail query containing tags, ordered images and real view/feedback counts.

Article creation/update, tags/images, publication timestamps and scrubbed audit metadata commit atomically. Updates/deletes compare the supplied version under an article row lock and increment it on update; stale versions return a conflict. Publication sets a timestamp, unpublishing clears it, and inactive categories cannot publish new/current content. Staff can retain or unpublish drafts in inactive categories. Ticket-linked article deletion is rejected; category management deactivates rather than deletes.

Media references are validated under the shared mutation lock: purpose must match, and a new attachment must belong to the actor or be approved by administrator authority. Existing same-article associations can be retained by another engineer editing that article. API limits are ten tags and ten images; ordered solutions contain one to 100 bounded text steps, subject to the whole 64 KiB JSON body limit.

Views and feedback use explicit authenticated/CSRF-protected operations, not GET side effects. Both require public article/category visibility even for staff metrics. View insertion and the 15-minute deduplication check serialize in a transaction. Feedback's unique `(user_id, article_id, attempt_id)` constraint is paired with immutable answer semantics: matching retries return no new event, conflicting retries return 409, and independent attempts get independent rows. These tables supply later analytics; no ticket is created by these operations.

## Ticket persistence (Phase 4 implemented)

- `tickets`: immutable sequence-backed number, title/description, requester, department snapshot, optional category, priority/status, nullable assignee, optimistic version, created/updated/resolved timestamps.
- `ticket_articles`: composite ticket/article key.
- `ticket_comments`: ticket, author, body, internal flag, timestamps.
- `ticket_attachments`: composite ticket/media key.
- `ticket_status_history`: ticket, previous/new status, actor, optional reason, timestamp.

Constraints reject invalid priorities/statuses and inconsistent resolution timestamps. The ticket service derives requester/department from current database identity, filters employee reads/writes by owner, hides internal comments, validates active engineer/administrator assignees and their departments, enforces transitions, and checks optimistic versions under a ticket row lock. History, actor, resolution timestamps, associations and scrubbed audit changes commit atomically using the shared management advisory lock and fresh actor rechecks. Comments/attachments also advance the ticket version. One resolved/closed ticket can create one linked unpublished article; draft conversion and ticket linkage commit together. Indexes cover requester/date, status/priority/date, assignee/status, department/category/date, and child foreign keys.

## Implemented media persistence (Phase 3)

`media_files` stores uploader, generated storage key, original display filename, approved content type, byte count, SHA-256 digest, purpose, and creation date. The database constrains purpose/type/size combinations and storage-key format. Bytes belong in the private persistent filesystem, never PostgreSQL.

[media.sql](../backend/db/queries/media.sql) implements metadata insertion and authorized lookup with a fresh active-user/role join. Uploader/administrator access is available independently of article publication; other engineers require an article association; any authenticated role may access media linked to published content in an active category. Unauthorized IDs are concealed as not found. Phase 4 adds `TICKET_ATTACHMENT` screenshot uploads for employees and ticket-linked requester/engineer download access. Another employee’s file ID is concealed as not found, including conditional/range requests.

The database permits image PNG/JPEG/WebP and video MP4/WebM within its hard purpose/size checks, but the current API accepts only PNG/JPEG (10 MiB) and MP4/WebM (100 MiB default, configurable up to the 500 MiB database bound). Frontend videos stay capped at 100 MiB. The actual upload bytes are sniffed, images fully decoded with bounded dimensions/pixels, and video containers structurally validated; no codec decoding or scanning is performed.

Storage uses random generated keys, private temporary files and an anchored filesystem root. Metadata creation rechecks current staff authority under the shared management lock and inserts `media.uploaded` audit metadata in the same PostgreSQL transaction. Known failures remove the file; an uncertain COMMIT result preserves it for reconciliation, avoiding deletion of potentially committed references. Database/filesystem changes are not a distributed atomic transaction: crashes can leave temporary/unreferenced files. Article replacement/deletion retains media metadata/bytes. Automatic orphan/unused-media cleanup, quotas and retention remain future operations work. Authorized GET/HEAD supports byte ranges and no-store delivery through the API, never a static volume alias.

## Audit semantics

Existing events cover login/logout, user creation/updates, own profile updates, department creation/updates, article creation/update/deletion, category creation/update, and media uploads. Metadata contains role/activation/department identifiers and password-reset/session-revocation flags as appropriate, never the password/hash, article text, original upload filename or full request body. Knowledge metadata uses article version/publication/category identifiers; media metadata records purpose, approved content type and byte count. View/feedback events use their dedicated tables. There are no API operations to mutate audit logs. Production runtime database privileges must support append-only audit writes and must be separated from migration privileges.

## Seed and migration verification

Migration 1 creates the base tables, constraints, indexes, timestamp triggers, search vectors, and roles; migration 2 adds article versions and the view deduplication index. `make seed-dev` explicitly adds idempotent synthetic department/category reference data with an `APP_ENV=development` gate.

`make demo-env` prepares missing random passwords in ignored local configuration; `make seed-accounts` runs the Go seed executable. It requires `APP_ENV=development`, hashes environment-provided passwords, seeds references, the three synthetic role accounts and safe NCALayer/printer/network guides atomically. Existing account password/role/active state and article content/publication/counters are preserved; tags are attached only to newly inserted demo articles. Production/test environments are rejected. Credentials are neither committed nor printed.

`make test-integration` creates a disposable PostgreSQL container with random credentials, applies all migrations through version 4, runs real repository/HTTP workflow tests with the race detector, applies reference seed twice, verifies migration rollback leaves no application tables, reapplies the schema, and reruns the suite. Fixtures are rolled back or explicitly cleaned up. Both complete Phase 3 repeats passed, including multilingual/tag-only relevance, draft/category visibility, version conflicts, event deduplication, media associations, audit rollback and the composed HTTP publication/media workflow. Live Docker/browser checks and repeated guide-seed fingerprints also passed; see the [delivery report](phase3-report.md). Never run destructive down migrations against clinic data. Backups must eventually include both PostgreSQL and media storage.

## Phase 5 reporting, audit and settings

`application_settings` has one constrained row: clinic name, default ticket priority, IANA reporting timezone, optimistic version and updated timestamp. Updates recheck the current administrator under the shared management lock, lock the row, compare/increment versions and insert `settings.updated` metadata in one transaction. Seeding never resets this row. Omitted ticket priority reads the current default within its existing authenticated creation transaction.

Staff reports and administrator audit reads use a read-only repeatable-read snapshot and recheck current actor authority. SQL separates view and feedback aggregates to prevent join multiplication. Ticket metrics describe the selected creation cohort’s current state; feedback/views retain their existing current user-department/article-category relationship. No historic department membership snapshot is fabricated. Default reporting dates resolve inside the same snapshot using configured timezone. Daily rows include zero-count days.

Audit browsing supports bounded date/actor/action/entity filters, stable time/ID ordering and pagination. Stored JSON is not trusted: responses keep only action-specific correctly typed UUIDs, enums, flags, counters and settings-field names. Unknown actions expose no metadata. No audit mutation endpoint exists. Restricted append-only runtime database privileges remain a deployment requirement.

## Migration 6: private school workspace

`school_workspace` binds a singleton to one existing account (no automatic grant). `school_assets` holds device inventory with optional inet addresses and version checks. `school_tickets` has independent numbering, status, priority, room, requester, category, device FK, lesson disruption and resolution timestamps. `school_history` stores private change metadata transactionally. None of these records join clinic reporting. Down migration deletes school data and is only for disposable tests.
