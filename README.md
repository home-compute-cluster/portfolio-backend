# portfolio-backend

`portfolio-backend` is the Go API for [packetcraft.dev](https://packetcraft.dev). It is a small modular monolith that will provide post reactions, visitor comments, administrator authentication, and project content for the static Astro frontend.

Iterations 0 and 1 provide the walking skeleton: typed configuration, PostgreSQL pooling, structured logging, liveness/readiness probes, graceful HTTP shutdown, CI, and a production-oriented container image.

## Prerequisites

- Go 1.26.4 or the version declared in `go.mod`
- access to the development K3s cluster and `kubectl`
- the existing CloudNativePG development cluster
- Docker or another OCI-compatible builder for container builds
- GNU Make (optional; every target is also a short Go command)

## Local configuration

Copy `.env.example` to `.env`, then replace the placeholder `DB_PASSWORD` in that ignored file. `DB_PASSWORD` overrides a password embedded in `DATABASE_URL`, which keeps the connection URL safe to show in logs and documentation. A complete production `DATABASE_URL` still works when `DB_PASSWORD` is unset.

PowerShell does not automatically load `.env`; either set both variables in the current terminal or use your IDE/environment loader. `Read-Host` can set the password without placing it in shell history:

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

## Run the API

With `DATABASE_URL` set in the current shell:

```powershell
go run ./cmd/api
```

The API listens on `:8080` by default:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/healthz
Invoke-RestMethod http://127.0.0.1:8080/api/readyz
```

`/api/healthz` checks only that the process is alive. `/api/readyz` pings PostgreSQL and returns `503` while it is unavailable.

## Verify changes

```powershell
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/api
```

On a system with Make, `make ci` runs the same core checks. Build the container with `docker build -t portfolio-backend:dev .`.

## Deployment model

CI validates the Go code and container build. The homelab GitOps repository remains the source of truth for the Kubernetes Deployment, Service, ConfigMap, Secret, IngressRoute, and CNPG resources. [`deploy/backend.example.yaml`](deploy/backend.example.yaml) is a reference showing the expected container security settings and probe wiring; it is not a second GitOps source of truth.

Traefik routes `packetcraft.dev/api/*` to this backend and all other paths to the frontend. Database outages fail readiness but never liveness, so Kubernetes removes an affected pod from service without restarting it in a loop.
