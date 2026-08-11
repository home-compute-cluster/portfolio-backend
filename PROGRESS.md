# Project progress

Last updated: 2026-08-10

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

## Iteration 3 — Content identity registry

Status: complete

Added or completed:

- added `content_items` as the backend authority for slug, kind, and publication state while leaving article bodies in Astro
- seeded the four currently published blog identities from the frontend repository
- constrained slug syntax/length, content kind, and publication state in both Go and PostgreSQL
- added a feature service and explicit pgx store for published-post checks
- documented forward-only migration updates as the synchronization boundary
- added unit and real-PostgreSQL tests for malformed, unknown, draft, archived, wrong-kind, and published identities

## Iteration 4 — Public comments

Status: complete

Added or completed:

- added plain-text comment persistence with rune-aware author/body validation and matching PostgreSQL constraints
- added `GET` and `POST /api/posts/{slug}/comments`
- added newest-first `before_id` cursor pagination with default and maximum page bounds
- added strict content-type/JSON decoding, unknown-field rejection, trailing-value rejection, and a 16 KiB request cap
- used a per-post transaction advisory lock so the visible-count check and insert remain atomic under concurrency
- kept visitor hashes and moderation state out of public responses
- updated typed configuration, `.env.example`, OpenAPI, deployment references, and application wiring

## Iteration 5 — Anonymous-write abuse controls

Status: assignment pending

Added or completed:

- added HMAC-SHA-256 visitor identity derived from normalized client IP and user agent without persisting either raw value
- added centralized trusted-proxy parsing that ignores forwarded headers from untrusted peers and safely handles malformed chains
- added a silent honeypot, request-size limits, strict validation, and an atomic per-post visible-comment cap
- added fuzz coverage for forwarded-header parsing and concurrent PostgreSQL coverage for the post cap
- added a rate-limiter interface at the HTTP boundary, a compile-ready assignment template, bounded-state requirements, and an opt-in race/concurrency acceptance suite

Remaining assignment:

- implement `internal/platform/ratelimit/AssignmentLimiter`
- make `make test-rate-limit-assignment` pass
- add typed comment/view/like allowance configuration and construct separate limiter instances in `internal/app`

The placeholder limiter is permissive and is intentionally not wired into the application. Iteration 5 must not be reported as complete until the assignment is implemented.

## Iteration 6 — Comment moderation

Status: complete but dormant

Added or completed:

- added bounded moderation listing with visible/hidden filters
- added idempotent explicit hide and unhide operations
- serialized visibility changes with comment creation so unhiding cannot exceed the visible-comment cap
- implemented service, PostgreSQL, and HTTP handler layers with unit, HTTP, and real-PostgreSQL tests
- deliberately left all moderation handlers out of the production router; an automated test verifies `/api/admin/comments` remains unavailable

Moderation will be registered only after admin session authentication can protect the entire route group.

## Iteration 7 — Post views

Status: complete

Added or completed:

- added `POST /api/posts/{slug}/view` with `204 No Content` for both newly counted and deduplicated views
- implemented a true rolling configurable window rather than calendar-day deduplication
- used a unique `(post_slug, visitor_hash)` row and one atomic PostgreSQL CTE to decide and increment cached totals
- kept writes synchronous and request-scoped with no untracked handler goroutines
- added deterministic clock injection plus HTTP, rolling-boundary, separate-visitor, and concurrent identical-visitor tests

## Automated verification for Iterations 3–7

Completed against an isolated PostgreSQL 18 instance:

- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go vet ./...`
- `go build ./cmd/api ./cmd/migrate`

The temporary PostgreSQL pod and its port-forward were removed after the run. Docker remains unavailable locally, so the unchanged container build continues to be verified by CI.

## Iteration 8 — Likes and public stats

Status: complete

Added or completed:

- added `PUT /api/posts/{slug}/like` and `DELETE /api/posts/{slug}/like` as explicit, idempotent desired-state operations
- added a composite `(post_slug, visitor_hash)` primary key and visitor-hash length constraint
- used `INSERT ... ON CONFLICT DO NOTHING` for atomic first/repeated likes and an idempotent conditional delete for unlikes
- added `GET /api/posts/{slug}/stats` returning only aggregate `views` and `likes`
- deliberately derived like totals with indexed `count(*)` rather than maintaining another cached counter that could drift
- kept the existing cached rolling-window view total and read both totals in one PostgreSQL statement snapshot
- added unit, HTTP, constraint, first/repeated operation, concurrency, stats-correctness, and unknown-post coverage
- updated application wiring, OpenAPI, README, and the rate-limit assignment handoff

The like limiter integration point exists but remains unwired with the other anonymous-write limiters until the Iteration 5 assignment is completed.

Automated verification completed against an isolated PostgreSQL 18 instance:

- formatting and module-tidiness checks
- `go vet ./...`
- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go build ./cmd/api ./cmd/migrate`

The disposable PostgreSQL pod and its `15433` port-forward were removed after verification. The developer-managed `15432` port-forward was not changed.

## Next recommended work

After completing the rate-limiter assignment, proceed to Iteration 9 for single-administrator authentication and protected admin route registration.
