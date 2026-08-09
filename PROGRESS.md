# Project progress

Last updated: 2026-08-09

## Iteration 0 — Repository baseline

Status: complete

Added or completed:

- established the documented `cmd`, `internal`, `api`, `deploy`, `migrations`, and `scripts` layout without creating speculative feature packages
- documented prerequisites, local development, the `15432` CNPG port-forward, tests, container builds, and the GitOps deployment boundary
- added a safe `.env.example`; real `.env` variants remain ignored
- added Make targets for formatting, module tidiness, vetting, normal/race tests, binary builds, and container builds
- added a multi-stage, non-root, shell-free container image
- added GitHub Actions checks for formatting, module tidiness, vetting, tests, race tests, the API build, and the container build
- added a minimal OpenAPI contract for the probe endpoints
- added a reference Kubernetes ConfigMap, Deployment, and Service with separate startup, liveness, and readiness probes

Verification completed locally:

- `go mod tidy`
- `gofmt` / formatting check
- `go vet ./...`
- `go test ./...`
- `go test -race ./...`
- `go build ./cmd/api`

The container build remains covered by CI because Docker is not installed in the current local environment.

## Iteration 1 — Application walking skeleton

Status: complete

Added or completed:

- typed, startup-validated HTTP and PostgreSQL configuration
- one shared `pgxpool.Pool` constructed at process startup
- JSON `log/slog` lifecycle and readiness-failure logging
- Chi routing and request ID/panic recovery middleware
- `GET /api/healthz`, independent of PostgreSQL
- `GET /api/readyz`, with a short database ping timeout and non-leaking `503` response
- explicit HTTP read-header, read, write, and idle timeouts
- `SIGINT`/`SIGTERM` handling with bounded graceful draining before pool closure
- tests for configuration validation, probe behavior, database error privacy/logging, unknown routes, panic recovery, and in-flight request draining

Automated verification:

- HTTP tests cover successful and unavailable readiness responses, liveness independence from PostgreSQL, error privacy, unknown routes, and panic recovery
- configuration and pool tests cover the separate password override and complete connection URLs
- CI runs the same behavior checks without requiring manual probe requests

### Readiness authentication follow-up

The readiness failure was traced to configuration semantics: `DATABASE_URL` contained the literal placeholder `password`, while a separate `DB_PASSWORD` value was not consumed by the application. The typed database configuration supports `DB_PASSWORD` as an explicit override, and tests verify that the override reaches pgx without embedding the secret in the URL.

Real passwords remain outside version control.

## Iteration 2 — Migrations and database test foundation

Status: complete

Added or completed:

- consecutive, forward-only `NNNNNN_name.sql` migrations
- an atomic pgx migration runner with a transaction-scoped PostgreSQL advisory lock
- a checksummed `schema_migrations` history that detects edited, removed, renamed, duplicate, or missing migrations
- a separate `cmd/migrate` entrypoint; API pods never migrate on startup
- an initial `000001_baseline.sql` migration without speculative feature tables
- an integration-test harness that creates a unique PostgreSQL schema per test and removes it automatically
- integration coverage for empty-database migration, idempotent reruns, rollback on failure, checksum enforcement, concurrent runners, and non-zero command failure status
- a CI PostgreSQL 18 service and explicit integration-test step
- API and migration binaries plus migration SQL in the container image
- a reference revision-specific Kubernetes migration Job

Automated verification:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./cmd/api ./cmd/migrate`
- PostgreSQL integration tests through `TEST_DATABASE_URL`

## Next recommended work

Iteration 3: add the lightweight content identity registry, its migration, synchronization boundary, and automated PostgreSQL/HTTP coverage.
