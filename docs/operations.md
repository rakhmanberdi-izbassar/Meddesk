# MedDesk operations and recovery

The delivered MVP is version 0.5.0 and requires clean migration version 7. Standard Compose is a loopback-only development deployment. Its PostgreSQL administrator credentials and HTTP ingress are development conveniences; production rollout is separate deployment work.

## Start, update and observe

Run `make env`, `make up`, `make seed-accounts`, `make smoke`, and `make smoke-auth` in development. Existing environment credentials, database/media volumes, seeded accounts/articles and administrator settings are preserved. Migration 4 adds the settings singleton and an analytics index without replacing existing data. The seed command requires explicit development mode and never provisions production accounts or resets clinic settings.

`make logs` shows normalized route/status/request-ID logs. `/api/v1/health` checks process liveness; `/ready` checks PostgreSQL and clean version 7. A failed/dirty migration requires investigation. Do not force a dirty version merely to pass readiness, or run application down migrations to repair it. Disposable migration rollback tests are available through `make test-integration`.

Only administrators change clinic name, default ticket priority and reporting timezone in `/admin/settings`. These are application preferences. Origins, session/cookie policy, database credentials and media limits stay in deployment configuration. Existing form priority choices remain stable after a setting change; omitted API priorities use the current database default inside ticket creation.

## Coordinated development backup

`make backup` creates a new private directory under ignored `backups/`. It briefly stops the development API, dumps PostgreSQL in custom format, archives private media from the same deployment, computes checksums, and restarts the API. Failures attempt to restart the original backend and mark the archive incomplete. Only a running development backend is accepted. No backup of the working application was created automatically during implementation.

For an explicit destination, use `bash scripts/backup-dev.sh /absolute/private/new-directory`. The destination must not already exist. Files include `database.dump`, `media.tar.gz`, `checksums.sha256`, and an archive notice. Environment configuration is not copied. Store credentials separately under the deployment’s secret-management process. Restrictive local permissions do not replace encrypted storage, encrypted transport, restricted access and an off-host copy. Archive data includes accounts/password hashes, sessions, tickets, feedback and private screenshots; treat it as confidential.

After copying an archive, run `sha256sum --check checksums.sha256` from its directory. Retain database and media archives together. Do not prune referenced media independently: article/ticket links may outlive publication or closure, and the application currently has no automated orphan reconciliation or retention job.

## Restore and exercise recovery

`make test-backup` creates isolated synthetic PostgreSQL 18 and media services, applies migrations 1–7, runs the backup command, checks checksums, imports into a separate empty database, extracts into a separate directory, and verifies real data, settings, schema version and media bytes. It also verifies that the source backend restarted, then removes only its disposable services/volumes/files. It never targets application volumes. See [the runnable restore test](../scripts/test-backup.sh).

For real recovery, first restore into a new isolated staging deployment with PostgreSQL compatible with the dump. Use `pg_restore --no-owner --no-privileges --exit-on-error` against an empty database and extract only a trusted media archive into an empty private media directory. Keep the old deployment intact until the restored copy is verified. Restore media ownership to the application UID (10001 in the provided image), private file permissions and the configured media directory. Checksums confirm archive integrity; they do not establish that all referenced files are present or operationally usable.

Before exposing a restored deployment, invalidate restored sessions, review account activation/roles and credentials against post-backup account changes, verify migration cleanliness/readiness, and exercise all three roles with synthetic test accounts. Test guide search, private image/video ranges, own-ticket isolation, internal comments, resolution/closure and reporting. Reconcile database media references with recovered bytes without blindly deleting files. Record the actual recovery time and data-loss window, then arrange application switching through the deployment owner’s change process.

## Production prerequisites assessed

The code validates production Secure cookies, explicit HTTPS origins and PostgreSQL `sslmode=verify-full`; standard Compose does not supply production TLS/certificate or secret management. Provide controlled HTTPS ingress/network access, a trusted certificate chain, URL-safe managed database credentials, a backed-up media volume and controlled initial administrator provisioning. Development demo provisioning is unavailable in production.

Separate migration and runtime PostgreSQL identities. Runtime access should support implemented application CRUD and migration-state reads while forbidding schema changes, truncation, and audit update/delete. Migration ownership/extension creation and narrowly granted runtime privileges require deployment validation; the implementation checks were run with disposable development database identities, not a production restricted role.

Set actual trusted proxy networks only when needed; avoid global CIDRs. Define backup frequency/retention, an off-host encrypted copy, recovery ownership, and tested recovery-point/recovery-time objectives. Define coordinated database/media retention, orphan reconciliation, upload quotas and malware/codec review before a clinic rollout. Current video validation checks containers, not decoding/transcoding/scanning. No patient records, signing keys, credentials or private certificates belong in this IT support application.

Monitor readiness failures, migration failures, disk/database capacity and backup age; retain normalized application logs under the organization’s policy. Audit browsing is read-only with action-specific typed metadata filtering; immutable audit writes still require database-level privilege separation in the deployed environment. No automatic retention/deletion or production deployment was performed as part of this milestone.
