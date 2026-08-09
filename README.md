# portfolio-backend

`portfolio-backend` is the Go API for [packetcraft.dev](https://packetcraft.dev). It is a small modular monolith that will provide post reactions, visitor comments, administrator authentication, and project content for the static Astro frontend.

Iterations 0 through 2 provide the walking skeleton and database foundation: typed configuration, PostgreSQL pooling, structured logging, liveness/readiness probes, graceful HTTP shutdown, versioned migrations, real-PostgreSQL integration tests, CI, and a production-oriented container image.

## Prerequisites

- Go 1.26.4 or the version declared in `go.mod`
- access to the development K3s cluster and `kubectl`
- the existing CloudNativePG development cluster
- Docker or another OCI-compatible builder for container builds
- GNU Make (optional; every target is also a short Go command)

## Local configuration

Copy `.env.example` to `.env`, then replace the placeholder `DB_PASSWORD` in that ignored file. `DB_PASSWORD` overrides a password embedded in `DATABASE_URL`, which keeps the connection URL safe to show in logs and documentation. A complete production `DATABASE_URL` still works when `DB_PASSWORD` is unset.

`cmd/api/main.go` loads `.env` automatically via `godotenv` before reading configuration, so a local `.env` file is picked up without any shell setup. This is a no-op in production, which has no `.env` file and relies on real environment variables from the GitOps-managed Secret instead. Variables already set in the shell still work and take precedence over `.env` if you prefer that route:

```powershell
$env:DATABASE_URL = "postgres://portfolio@127.0.0.1:15432/portfolio?sslmode=require"
$env:DB_PASSWORD = Read-Host "Database password"
go run ./cmd/api
```

The only universally required variable is `DATABASE_URL`; `DB_PASSWORD` is an optional explicit password override for local development. Local development expects the existing CNPG read-write Service to be available at `127.0.0.1:15432` through a developer-managed port-forward:

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

## Automated verification

Every push and pull request runs formatting, module-tidiness, vet, unit/HTTP tests, real-PostgreSQL integration tests, the race detector, both Go binary builds, and a container build. Developers are not expected to reproduce routine API behavior manually.

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
