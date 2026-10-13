# Phase 1 delivery report

Completed on 2026-10-09 in the initially empty HelpDesk workspace. A local Git repository was initialized; no remote, commit, or publication was created.

## Implemented

The four requested design documents were written before application implementation. The foundation uses Go 1.27.2, pgx, generated sqlc queries, PostgreSQL 18, React/TypeScript/Vite, Tailwind, shadcn/ui primitives, Router, Query, Zod, Fetch, and Lucide. React Hook Form dependencies are prepared for authenticated forms in Phase 2.

The API provides process liveness, real database/schema readiness, safe application metadata, and embedded OpenAPI JSON. It includes constructor injection, configuration validation, structured JSON errors and logs, internally generated request IDs, request/database deadlines, security headers, and graceful shutdown. Production configuration requires PostgreSQL TLS verification.

The normalized schema includes identity/session, knowledge, ticket, media metadata, and audit tables, foreign keys, check/unique constraints, indexes, timestamp triggers, immutable ticket numbers, weighted Kazakh-friendly/simple and Russian search vectors, and trigram indexes. These are schema foundations; domain APIs and ranked search remain scheduled work.

The responsive frontend has a sidebar/top navigation, persisted Kazakh/Russian selection, live API/database status with loading/error/refresh states, accessible mobile navigation, and an explicitly unavailable login screen. It contains no simulated accounts, tickets, or analytics.

Compose starts PostgreSQL, a one-shot migration job, nonroot backend/frontend containers, and a nonroot Nginx gateway. Only the loopback development gateway is exposed by default. Named PostgreSQL and private media volumes persist data. Environment initialization generates local credentials into an ignored mode-0600 `.env`; source contains no usable credentials. A development-gated reference seed creates synthetic departments/categories only.

## Executed checks

- `make test`: passed. Go race tests and `go vet ./...`; frontend clean `npm ci`, TypeScript checks, 13 component/API-client tests across two suites, and Vite production build. Ordinary Go tests explicitly skip PostgreSQL tests without `TEST_DATABASE_URL`; the integration command below executes them against an actual database.
- `make test-integration`: passed on the final schema and pgx version. Disposable PostgreSQL 18 migrations up, constraint/link/timestamp/search-vector/feedback tests with race detection, two seed executions, complete rollback with no application tables left, reapply, and the same database tests again. Temporary database/container removed afterward.
- `make generate`: passed as the normal project user. SHA-256 comparisons confirmed all generated files remained identical after regeneration with sqlc 1.31.1.
- `make compose-check` and `bash -n scripts/*.sh`: passed.
- `make up`: both application images built; migration job completed; PostgreSQL, backend, frontend, and Nginx all became healthy.
- `make smoke`: all four API endpoints plus `/` and `/login` returned HTTP 200 through the gateway.
- Additional gateway assertions passed: valid JSON bodies, non-cacheable API responses, `nosniff`, server-generated request IDs, structured 404 and 405 errors, OpenAPI JSON, and absent cross-origin permission headers.
- Database outage check passed: after stopping development PostgreSQL, liveness remained 200 and readiness returned 503. PostgreSQL was restarted and readiness recovered to 200.
- A synthetic query marker was absent from backend logs; raw query strings and untrusted request IDs are not logged.
- Runtime checks confirmed backend UID/GID 10001, frontend UID/GID 101, and writable private media storage. A temporary media marker survived backend container recreation and was removed. Seeded 3 departments and 5 categories remained after PostgreSQL restart; there are no development accounts.
- Desktop 1440px, mobile 390px, and login screenshots from the running gateway were inspected. The overview showed actual healthy responses without visible overflow; login clearly identified pending authentication. React tests cover locale switching/persistence and mobile navigation. Connected-browser automation was unavailable; screenshot inspection used headless Chrome.

The GitHub Actions workflow was added, but no remote CI run was triggered. Its underlying local commands were executed as described above. Full security/authentication, upload, business workflow, and search-ranking tests belong to the corresponding future phases; they are not claimed complete.

## Issues corrected

- Docker sqlc initially created root-owned generated files. Ownership was corrected; `make generate` explicitly uses the invoking user's UID/GID and was verified afterward.
- TypeScript's removed `baseUrl` option caused the initial type check to fail. Path configuration was corrected and type checks, tests, and build passed afterward.
- Review tightened bcrypt storage constraints to valid costs 04–31 and added an invalid-cost integration test. Migration tests were rerun on the final constraint, and the already-created local foundation database was aligned with it before delivery.
- Native development media storage is Git-ignored. Nginx request logs are suppressed because raw URLs can contain sensitive query values; API structured logs use normalized routes.

## Verify locally

The stack is currently running at [http://localhost:8080](http://localhost:8080).

```bash
make env
make up
make seed-dev
make smoke
make test
make test-integration
```

Inspect the live status cards, switch between Kazakh and Russian, and open `/login` to see the Phase 2 notice. API documentation is at `/api/v1/openapi.json`. `make down` stops the stack while retaining named volumes. See [README](../README.md) for prerequisites, native development, and environment details.

## Changed files

All listed application files are new; design documents were updated during implementation. Existing `.idea/` metadata was left intact and excluded from Git.

```text
.env.example
.gitignore
.github/workflows/ci.yml
Makefile
README.md
docker-compose.yml
docker-compose.dev.yml
docs/architecture.md
docs/requirements.md
docs/database.md
docs/implementation-plan.md
docs/phase1-report.md
infra/nginx/default.conf
scripts/init-env.sh
scripts/smoke.sh
scripts/test-integration.sh
backend/.dockerignore
backend/Dockerfile
backend/go.mod
backend/go.sum
backend/sqlc.yaml
backend/cmd/api/main.go
backend/api/openapi/embed.go
backend/api/openapi/openapi.json
backend/db/migrations/000001_initial_schema.up.sql
backend/db/migrations/000001_initial_schema.down.sql
backend/db/queries/health.sql
backend/db/sqlc/migration_state.sql
backend/db/seeds/development.sql
backend/internal/config/config.go
backend/internal/config/config_test.go
backend/internal/middleware/middleware.go
backend/internal/middleware/middleware_test.go
backend/internal/platform/db/db.go
backend/internal/platform/db/models.go
backend/internal/platform/db/querier.go
backend/internal/platform/db/health.sql.go
backend/internal/platform/health/service.go
backend/internal/platform/health/service_test.go
backend/internal/platform/health/handler.go
backend/internal/platform/health/handler_test.go
backend/internal/platform/postgres/pool.go
backend/internal/platform/requestmeta/requestmeta.go
backend/internal/platform/response/response.go
backend/tests/schema_integration_test.go
frontend/.dockerignore
frontend/Dockerfile
frontend/default.conf
frontend/nginx.conf
frontend/package.json
frontend/package-lock.json
frontend/components.json
frontend/index.html
frontend/tsconfig.json
frontend/tsconfig.app.json
frontend/tsconfig.node.json
frontend/vite.config.ts
frontend/public/favicon.svg
frontend/src/App.tsx
frontend/src/main.tsx
frontend/src/styles.css
frontend/src/components/Shell.tsx
frontend/src/components/ui/button.tsx
frontend/src/components/ui/badge.tsx
frontend/src/lib/api.ts
frontend/src/lib/locale.tsx
frontend/src/lib/utils.ts
frontend/src/pages/Overview.tsx
frontend/src/pages/Login.tsx
frontend/src/test/setup.ts
frontend/src/test/App.test.tsx
frontend/src/test/api.test.ts
```

## Next phase

Phase 2: bcrypt authentication, hashed opaque cookie sessions, CSRF, login rate limiting, RBAC, profile, employee/department management, protected frontend navigation, and development-only demo accounts with externally supplied passwords. The delivered foundation stops at the requested Phase 1 boundary.
