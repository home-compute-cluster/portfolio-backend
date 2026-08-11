# portfolio-backend

`portfolio-backend` is the Go API for [packetcraft.dev](https://packetcraft.dev). It is a small modular monolith that provides post reactions, visitor comments, and Access-protected moderation for the static Astro frontend. Authored content remains in Astro/Git rather than PostgreSQL.

The implemented walking skeleton and feature slices provide typed configuration, PostgreSQL pooling, structured logging, liveness/readiness probes, graceful HTTP shutdown, versioned migrations, a content registry, public comments, visitor privacy controls, real-PostgreSQL integration tests, CI, and a production-oriented container image.

## Prerequisites

- Go 1.26.4 or the version declared in `go.mod`
- access to the development K3s cluster and `kubectl`
- the existing CloudNativePG development cluster
- Docker or another OCI-compatible builder for container builds
- GNU Make (optional; every target is also a short Go command)

## Local configuration

Copy `.env.example` to `.env`, then replace the `DB_PASSWORD` and `VISITOR_HMAC_KEY` placeholders in that ignored file. The HMAC key must contain at least 32 random bytes and must remain stable across replicas and restarts; changing it changes every pseudonymous visitor identity. `DB_PASSWORD` overrides a password embedded in `DATABASE_URL`, which keeps the connection URL safe to show in logs and documentation. A complete production `DATABASE_URL` still works when `DB_PASSWORD` is unset.

`cmd/api/main.go` loads `.env` automatically via `godotenv` before reading configuration, so a local `.env` file is picked up without any shell setup. This is a no-op in production, which has no `.env` file and relies on real environment variables from the GitOps-managed Secret instead. Variables already set in the shell still work and take precedence over `.env` if you prefer that route:

```powershell
$env:DATABASE_URL = "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require"
$env:DB_PASSWORD = Read-Host "Database password"
go run ./cmd/api
```

The migration command requires `DATABASE_URL`; the API additionally requires `VISITOR_HMAC_KEY`. `DB_PASSWORD` is an optional explicit password override for local development. Local development expects the existing CNPG read-write Service to be available at `127.0.0.1:15432` through a developer-managed port-forward:

```powershell
kubectl -n portfolio-dev port-forward service/portfolio-db-dev-rw 15432:5432
```

Production pods do not use a port-forward. Their `DATABASE_URL` points directly at the CNPG `-rw` Service DNS name and is supplied by the GitOps-managed Secret.

## Local development

With `DATABASE_URL` set in the current shell:

```powershell
go run ./cmd/api
```

The API listens on `:8080` by default. Automated HTTP tests verify `/api/healthz`, `/api/readyz`, unknown routes, middleware recovery, and database-error privacy; manual endpoint probing is not part of routine acceptance.

The currently usable dynamic routes are:

```text
GET  /api/posts/{slug}/comments
POST /api/posts/{slug}/comments
POST /api/posts/{slug}/view
PUT  /api/posts/{slug}/like
DELETE /api/posts/{slug}/like
GET  /api/posts/{slug}/stats
```

Only slugs in the published content registry are accepted. Comments are plain text, newest-first, and cursor-paginated. Views use a rolling deduplication window. Likes use explicit idempotent desired-state operations, and stats expose only aggregate view and like totals. Comment listing, hiding, and unhiding are registered only as one Cloudflare Access-protected `/api/admin` route group; successful state changes write a minimal audit event in the same PostgreSQL transaction.

The in-memory rate-limiting algorithm is the remaining Iteration 5 assignment. Its handler boundary and opt-in acceptance suite are present, but the permissive template is not wired into the API. Run `make test-rate-limit-assignment` while implementing it; do not treat comment, view, or like writes as fully hardened until that target passes and separate limiters are constructed in `internal/app`.

## Automated verification

Every push and pull request runs formatting, module-tidiness, vet, unit/HTTP tests, real-PostgreSQL integration tests, the race detector, all Go command builds, and a container build. Developers are not expected to reproduce routine API behavior manually.

CI also runs pinned Staticcheck and govulncheck releases plus a high/critical container vulnerability scan. Post-deployment behavior is covered by the automated smoke command in `cmd/smoke`; incident procedures and the production security review live in `docs/runbooks.md` and `docs/security.md`.

The same automated checks can be invoked locally when needed:

```powershell
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/api ./cmd/migrate
```

On a system with Make, `make ci` runs checks that do not require a local PostgreSQL instance. `make test-integration` uses `TEST_DATABASE_URL`; CI always supplies its own PostgreSQL service automatically.

## Database migrations

Versioned SQL lives in `migrations/`. The API never migrates on startup. The same container image includes `/migrate` and `/migrations` so GitOps can run one revision-specific migration Job before rolling out API pods. See [`migrations/README.md`](migrations/README.md) for the file convention and guarantees.

## Deployment model

CI validates the Go code and container build. The homelab GitOps repository remains the source of truth for the Kubernetes Deployment, Service, ConfigMap, Secret, IngressRoute, and CNPG resources. [`deploy/backend.example.yaml`](deploy/backend.example.yaml) is a reference showing the expected container security settings and probe wiring; it is not a second GitOps source of truth.

Traefik routes `packetcraft.dev/api/*` to this backend and all other paths to the frontend. Database outages fail readiness but never liveness, so Kubernetes removes an affected pod from service without restarting it in a loop.

`TRUSTED_PROXY_CIDRS` must contain only the actual Traefik and intermediate proxy network ranges. With it unset, forwarded headers are ignored and the direct peer is used. The application never trusts `X-Forwarded-For` from an arbitrary client, and Traefik must independently be configured with its own `forwardedHeaders.trustedIPs` boundary.

Administrator authentication is owned by Cloudflare Access rather than an application password or session system. The API requires `CF_ACCESS_TEAM_DOMAIN`, `CF_ACCESS_AUD`, and `ADMIN_EMAIL`, fetches rotating RSA signing keys from the team certificates endpoint, and independently validates every assertion before protected handlers can run. These are public verification parameters; no Access assertion or cookie is stored or logged.
