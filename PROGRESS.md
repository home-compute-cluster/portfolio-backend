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

Status: implementation complete; live database acceptance is pending a valid local secret

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

Local runtime verification:

- the API started and `/api/healthz` returned `200` with `{"status":"ok"}`
- the user-managed port-forward on `127.0.0.1:15432` was reachable
- CNPG reported `portfolio-db-dev` healthy with one ready instance
- `/api/readyz` correctly returned a non-leaking `503` for the placeholder `password`; the real database password was not read or logged

After a valid local `DATABASE_URL` is supplied, the remaining acceptance check is a live `/api/readyz` response of `200` with `{"status":"ready"}`.

## Next recommended work

Iteration 2: introduce versioned migrations, a single migration command, and PostgreSQL integration-test infrastructure. No feature tables or migration framework have been added early.
