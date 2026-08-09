# AGENTS.md

## Purpose

This repository contains the Go backend for `packetcraft.dev`.

The frontend that will be calling this backend is on https://github.com/BarneyLaw/portfolio-site

The backend serves the dynamic features used by the static Astro frontend:

- post view counts
- post likes
- visitor comments
- admin authentication
- admin-managed project content

The application runs in Kubernetes/K3s behind Traefik and Cloudflare Tunnel and connects to a CloudNativePG-managed PostgreSQL cluster.

The goal is a small, production-quality **modular monolith**. Prefer explicit, boring, maintainable Go over abstractions, frameworks, or distributed systems that are not required.

## Core engineering principles

1. Prefer the Go standard library.
2. Use `net/http` for HTTP.
3. Use `chi` for routing.
4. Use `pgx/v5` and `pgxpool` for PostgreSQL.
5. Use `log/slog` for structured logging.
6. Keep the application as one deployable API binary unless there is a demonstrated need to split it.
7. Keep business rules independent from HTTP and PostgreSQL details.
8. Prefer small interfaces declared at the point of use.
9. Do not introduce interfaces for every concrete type.
10. Do not create generic repository abstractions such as `Create(Entity)` or `FindByID(any)`.
11. Prefer feature-oriented packages.
12. Make invariants explicit in Go validation and PostgreSQL constraints where appropriate.
13. Use transactions whenever correctness spans multiple SQL statements.
14. Propagate `context.Context` through every request and database operation.
15. Treat security, observability, testing, deployment, and migrations as part of application design.
16. Do not add infrastructure because it might be useful someday.
17. Optimize for maintainability and correctness before cleverness.

## Target repository layout

```text
.
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── migrate/
│       └── main.go
├── internal/
│   ├── app/
│   ├── config/
│   ├── httpapi/
│   │   └── middleware/
│   ├── comments/
│   │   └── postgres/
│   ├── reactions/
│   │   └── postgres/
│   ├── projects/
│   │   └── postgres/
│   ├── adminauth/
│   │   └── postgres/
│   ├── content/
│   └── platform/
│       ├── postgres/
│       └── clock/
├── migrations/
├── api/
│   └── openapi.yaml
├── deploy/
├── scripts/
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

Do not create empty feature packages in advance. Add packages when implementing the feature.

## Dependency direction

The normal dependency direction is:

```text
HTTP handler
    ↓
application service
    ↓
small store interface
    ↓
PostgreSQL implementation
```

HTTP handlers may know about HTTP. PostgreSQL packages may know about `pgx`. Application services should not know about HTTP status codes, cookies, routers, SQL strings, or `pgx`.

Business errors should be application-level errors and mapped to HTTP responses in the HTTP layer.

## Application entrypoint

`cmd/api/main.go` should stay small. It may:

1. load configuration
2. construct the logger
3. construct the PostgreSQL pool
4. construct services
5. construct the HTTP router
6. construct `http.Server`
7. start the server
8. respond to `SIGINT` and `SIGTERM`
9. perform graceful shutdown
10. close resources

Do not place business logic in `main.go`.

## Configuration

All environment-variable parsing belongs in `internal/config`.

The rest of the application receives typed configuration values. Do not call `os.Getenv` from arbitrary feature packages.

Configuration must be validated at startup.

Typical configuration includes:

```text
HTTP_ADDR
HTTP_READ_HEADER_TIMEOUT
HTTP_READ_TIMEOUT
HTTP_WRITE_TIMEOUT
HTTP_IDLE_TIMEOUT
HTTP_SHUTDOWN_TIMEOUT

DATABASE_URL
DB_MAX_CONNS
DB_MIN_CONNS
DB_CONNECT_TIMEOUT
DB_QUERY_TIMEOUT
DB_READINESS_TIMEOUT

MAX_COMMENT_CHARS
MAX_COMMENTS_PER_POST

RATE_COMMENTS_PER_MIN
RATE_LIKES_PER_MIN
RATE_READS_PER_MIN
RATE_LOGIN_PER_15MIN

VIEW_DEDUP_WINDOW_HOURS
SESSION_TTL_HOURS

ADMIN_PASSWORD_HASH
VISITOR_HMAC_KEY
```

Never log complete connection strings, passwords, session tokens, or HMAC keys.

## PostgreSQL

Use `pgxpool.Pool`.

Create the pool once during application startup and share it. Do not create a new pool per request.

Every query must receive a `context.Context`.

Prefer explicit SQL over ORM abstractions.

SQL belongs in the PostgreSQL implementation package for the relevant feature.

Use PostgreSQL capabilities where they improve correctness:

- `CHECK` constraints
- foreign keys
- unique constraints
- partial indexes
- transactions
- `RETURNING`
- `ON CONFLICT`
- advisory locks where justified
- appropriate transaction isolation where justified

Do not depend only on Go validation for persistent invariants.

### Development database

For local development, the API may run on the developer machine while PostgreSQL runs inside the Kubernetes development cluster.

Expected workflow:

```text
local Go process
    ↓
127.0.0.1:15432
    ↓ kubectl port-forward
CloudNativePG -rw Service
    ↓
PostgreSQL primary
```

Prefer the CNPG read-write Service:

```text
<cluster-name>-rw
```

not `-r` or `-ro`, because this application performs writes.

Example:

```powershell
kubectl -n portfolio-dev port-forward service/portfolio-db-dev-rw 15432:5432
```

A port-forward is a development convenience only. Production pods connect directly to the Kubernetes Service DNS name.

If a development port-forward is unstable, it is acceptable to wrap `kubectl port-forward` in a retry loop. Do not build application-level logic around the existence of a port-forward.

```powershell
while true; do
    kubectl port-forward -n portfolio-dev svc/portfolio-db-dev-rw 15432:5432;
    echo "forward died, restarting...";
    sleep 1;
done
```

All the information you may need regarding the DB is at .env.example, and DO NOT read the value of the password directly, just use its reference "password"
Inform the user directly if the database cannot be accessed

## Health and readiness

Keep liveness and readiness separate.

### `GET /api/healthz`

Purpose: indicate that the API process is alive.

Rules:

- must not query PostgreSQL
- must not depend on external services
- should return quickly
- returns `200 OK` when the process is healthy

Expected body:

```json
{"status":"ok"}
```

### `GET /api/readyz`

Purpose: indicate that this instance can currently serve application traffic.

Rules:

- perform a PostgreSQL `Ping`
- use a short context timeout
- return `200 OK` when PostgreSQL is reachable
- return `503 Service Unavailable` otherwise
- never expose the underlying PostgreSQL error to the client
- log the underlying error internally

Expected success:

```json
{"status":"ready"}
```

Expected failure:

```json
{"status":"unavailable"}
```

Do not rate-limit Kubernetes probe endpoints.

Do not make `/healthz` depend on the database. A PostgreSQL outage should remove the pod from readiness, not cause Kubernetes to repeatedly restart the API.

## HTTP server

Use `http.Server` with explicit:

```text
ReadHeaderTimeout
ReadTimeout
WriteTimeout
IdleTimeout
```

Use graceful shutdown with `http.Server.Shutdown`.

On shutdown:

1. receive `SIGTERM` or `SIGINT`
2. stop accepting new work
3. allow in-flight requests to drain within a deadline
4. close the PostgreSQL pool
5. exit

Do not use `os.Exit` from deep inside application packages.

## HTTP API conventions

Keep application endpoints under `/api`.

Current intended surface:

```text
GET    /api/healthz
GET    /api/readyz

GET    /api/posts/{slug}/stats
GET    /api/posts/{slug}/comments
GET    /api/projects
GET    /api/projects/{slug}

POST   /api/posts/{slug}/view
PUT    /api/posts/{slug}/like
DELETE /api/posts/{slug}/like
POST   /api/posts/{slug}/comments

POST   /api/admin/login
POST   /api/admin/logout
GET    /api/admin/comments
POST   /api/admin/comments/{id}/hide
POST   /api/admin/comments/{id}/unhide
POST   /api/admin/projects
PUT    /api/admin/projects/{slug}
DELETE /api/admin/projects/{slug}
```

Prefer explicit desired-state operations. Collection endpoints must be bounded. Prefer cursor pagination for growing collections.

## Trust tiers

Every endpoint belongs to one trust tier.

### Public read

Protection: rate limiting where appropriate.

### Anonymous write

Protection:

- rate limiting
- request-size limits
- strict validation
- honeypot where applicable
- per-post abuse controls

### Admin

Protection:

- session authentication
- rate limiting
- origin/cross-site protections
- audit logging

All admin routes must live under `/api/admin/*`.

Apply admin authentication to the route group so handlers cannot accidentally omit it.

## Client IP and proxy trust

Traffic reaches the application through Cloudflare Tunnel and Traefik.

Never trust arbitrary forwarded headers directly from the public internet.

The ingress path must be configured so trusted proxy headers are accepted only from trusted upstream infrastructure.

Do not blindly trust `X-Forwarded-For` or `CF-Connecting-IP` unless the request path guarantees they came from the expected proxy.

Client-IP parsing should be centralized in middleware and covered by unit tests.

## Visitor identity

Anonymous abuse controls may use a pseudonymous visitor identifier.

Do not store raw IP addresses or raw user agents solely for this purpose.

Preferred construction:

```text
HMAC-SHA-256(secret, normalized-IP || separator || normalized-user-agent)
```

Store the result as binary (`BYTEA`) where practical.

Treat this identity as best-effort abuse prevention, not authentication.

## Comments

Comments publish immediately and may later be hidden by the administrator.

States:

```text
visible
hidden
```

Prefer soft hiding over deletion.

Validate author and body in Go and reinforce important limits with PostgreSQL constraints.

Visitor comments are plain text, not trusted HTML.

A `COUNT` followed by `INSERT` is race-prone. The comment-limit check and insert must be implemented as one database correctness boundary using appropriate locking or isolation.

Prefer a store operation such as:

```go
CreateVisibleIfUnderLimit(...)
```

rather than exposing separate count and insert operations.

## Views

Define view deduplication precisely.

Do not claim a `DATE` key implements a rolling 24-hour window.

Choose either calendar-day deduplication or rolling-window deduplication.

For a rolling window, store timestamps and perform deduplication and increment atomically.

Do not launch untracked goroutines from HTTP handlers for database writes.

## Likes

Likes are unique per visitor and post.

Use a unique constraint on:

```text
(post_slug, visitor_hash)
```

Prefer:

```text
PUT    /api/posts/{slug}/like
DELETE /api/posts/{slug}/like
```

Adding a like must not increment a count if the row already exists. Removing a like must not decrement a count if no row exists.

If cached totals are maintained, update the unique row and total atomically.

## Admin authentication

There is one administrator.

Do not introduce OAuth, JWT refresh-token systems, or user-management machinery unless requirements change.

Use:

- one Argon2id password hash
- opaque random session tokens
- server-side sessions
- hashed session tokens in PostgreSQL
- secure cookies

Cookie expectations:

```text
HttpOnly
Secure
SameSite=Strict
Path=/
Domain omitted
```

Prefer a `__Host-` cookie name where compatible.

Never store or log raw session tokens or the administrator password.

Admin write requests should have an additional cross-site request defense such as `Origin` validation or Fetch Metadata validation.

## Projects

Recommended states:

```text
draft
published
archived
```

Prefer optimistic locking using a version column so stale browser tabs cannot silently overwrite newer edits.

Treat Markdown-to-HTML rendering as a security boundary. Do not enable raw HTML without a deliberate sanitization strategy.

## Content identity

The backend must not accept arbitrary post slugs indefinitely without validating that the content exists.

The Astro frontend remains responsible for static article content.

The backend may keep a lightweight registry containing valid post/project identities and publication state.

Do not move static blog bodies into PostgreSQL unless requirements change.

## Error handling

Do not leak internal errors to clients.

Never return:

- raw PostgreSQL errors
- SQL
- stack traces
- constraint names
- internal package paths
- passwords
- connection strings

Suggested statuses:

```text
400 malformed or invalid request
401 missing or invalid admin session
404 resource not found
409 state conflict
413 request body too large
429 rate limited
500 unexpected internal error
503 dependency unavailable
```

Unexpected errors must be logged with enough context to debug them.

## Request validation

Validate at the boundary.

For JSON endpoints:

- cap request size
- verify expected content type where appropriate
- decode into explicit request structs
- reject malformed JSON
- reject trailing JSON values
- consider rejecting unknown fields

Remember that Go `len(string)` counts bytes, not Unicode characters. If a requirement is expressed in characters, use rune-aware counting.

Trim user-facing text and reject values that contain only whitespace.

## Logging

Use `log/slog`.

Useful request fields:

```text
request_id
method
route
status
duration
response_bytes
authenticated_admin
error_category
```

Do not log:

```text
comment bodies
project bodies
passwords
cookies
raw session tokens
raw IP addresses
raw user agents
complete visitor hashes
complete database URLs
```

Log the event, not sensitive payloads.

## Database migrations

The application owns its schema. ArgoCD/CNPG owns the PostgreSQL cluster resources.

Use versioned SQL migrations.

Do not modify production schema manually as part of normal development.

Do not rely on every application pod independently racing to run migrations.

Prefer one migration execution per deployment, for example a Kubernetes Job.

Prefer expand-and-contract schema changes for compatibility with rollout and rollback.

## Testing strategy

Testing is part of feature completion.

### Unit tests

Use for validation, service behavior, error mapping, client-IP parsing, visitor HMAC behavior, session expiry, pagination parsing, and state transitions.

Test observable behavior rather than internal method-call sequences.

### HTTP tests

Use `httptest` against the real router and middleware composition.

Test status codes, JSON responses, invalid payloads, body limits, authentication boundaries, cookie flags, panic recovery, admin route protection, readiness behavior, and proxy-header behavior.

### PostgreSQL integration tests

Use real PostgreSQL. Do not substitute SQLite for PostgreSQL correctness tests.

Test migrations, constraints, transactions, concurrency behavior, comment limits, view deduplication, like idempotency, session expiry, and optimistic project updates.

### Race detector

Run:

```bash
go test -race ./...
```

in CI.

### Fuzz testing

Use Go fuzz tests where hostile parsing input is relevant, especially client-IP/proxy headers, pagination cursors, slugs, and request decoding.

## CI expectations

A pull request should eventually run at least:

```text
gofmt check
go vet ./...
staticcheck ./...
go test ./...
go test -race ./...
govulncheck ./...
integration tests
go build ./cmd/api
docker build
```

Do not merge code that does not compile or whose tests fail.

## Security headers and CORS

The frontend and API use the same origin.

Do not add permissive CORS configuration unless the deployment architecture changes.

Security headers should have one clear owner. If Traefik owns them, do not add conflicting duplicates in Go.

## Rate limiting

An in-memory rate limiter is acceptable initially.

Document its limits:

- state is local to one pod
- limits reset on restart
- multiple replicas multiply effective limits
- key storage must be bounded

Do not add Redis solely for rate limiting at current scale.

## Background work

Do not start untracked goroutines from handlers for durable work.

If future features such as email notifications require reliable background processing, use a durable mechanism such as a transactional outbox.

Do not add Kafka, RabbitMQ, NATS, or Redis Streams without a demonstrated requirement.

## Kubernetes expectations

Traefik owns `/api` routing.

The frontend nginx container does not proxy the API.

Expected routing:

```text
packetcraft.dev/api/* -> portfolio-backend
packetcraft.dev/*     -> portfolio-frontend
```

Backend pods should use:

```text
startupProbe   -> /api/healthz
livenessProbe  -> /api/healthz
readinessProbe -> /api/readyz
```

A database outage should cause readiness failure but should not cause liveness failure.

## Deployment ownership

```text
ArgoCD / homelab config:
    Deployment
    Service
    IngressRoute
    ConfigMap
    Sealed Secret
    CNPG cluster resources

application repository:
    Go source
    migrations
    Dockerfile
    API contract
    tests
```

Secrets must not be committed to Git.

## What not to introduce without explicit justification

Avoid adding these by default:

- Gin
- Fiber
- Echo
- GORM
- Ent
- dependency-injection frameworks
- generic repository frameworks
- microservices
- Redis
- Kafka
- RabbitMQ
- NATS
- Elasticsearch
- JWT refresh-token machinery
- OAuth for the single-admin case
- GraphQL
- CQRS
- event sourcing

This is not a permanent ban. If a future requirement clearly benefits from one, document the problem first and explain why the added complexity is justified.

## Working style for coding agents

When implementing a task:

1. Inspect the existing repository before editing.
2. Preserve existing conventions unless there is a correctness reason to change them.
3. Make the smallest coherent change that satisfies the requirement.
4. Do not refactor unrelated code.
5. Do not create speculative abstractions.
6. Add or update tests with the feature.
7. Run formatting and relevant tests before reporting completion.
8. For database changes, add a migration.
9. For API changes, update `api/openapi.yaml` when present.
10. For configuration changes, update `.env.example` and documentation.
11. Never hard-code secrets.
12. Never weaken authentication, proxy trust, TLS, or validation just to make a local test pass.
13. When behavior is ambiguous, prefer the simplest interpretation consistent with this file and existing code.
14. If a decision has significant long-term consequences, record it as an ADR rather than silently embedding it in code.

## Initial implementation order

### Phase 1 — walking skeleton

Implement:

- typed configuration
- structured logger
- PostgreSQL pool
- `/api/healthz`
- `/api/readyz`
- HTTP server timeouts
- graceful shutdown
- basic tests
- container build

Definition of done:

```text
API starts with valid configuration
/api/healthz returns 200 without querying PostgreSQL
/api/readyz returns 200 when PostgreSQL is reachable
/api/readyz returns 503 when PostgreSQL is unavailable
PostgreSQL errors are logged but not returned to clients
SIGTERM shuts down cleanly
go test ./... passes
go test -race ./... passes
container image builds
```

### Phase 2 — comments

Implement comments end-to-end:

- migration
- model
- store interface
- PostgreSQL implementation
- service
- HTTP handlers
- validation
- visitor HMAC
- abuse controls
- pagination
- admin hide/unhide
- tests

### Phase 3 — reactions

Implement views, likes/unlikes, stats, and concurrency tests.

### Phase 4 — admin authentication

Implement password verification, opaque sessions, secure cookies, login rate limiting, session expiry, logout, admin middleware, and audit events.

### Phase 5 — projects

Implement CRUD, publication states, optimistic locking, safe Markdown rendering, and API tests.

### Phase 6 — operational hardening

Add metrics, CI security checks, deployment smoke tests, restore testing, and runbooks as needed.

## Source guidance

For security-sensitive, database-sensitive, Kubernetes-sensitive, or current library behavior, verify against reputable primary sources.

Preferred references:

- Go documentation: `go.dev` and `pkg.go.dev`
- PostgreSQL documentation: `postgresql.org/docs`
- Kubernetes documentation: `kubernetes.io/docs`
- CloudNativePG documentation: `cloudnative-pg.io/docs`
- Traefik documentation: `doc.traefik.io`
- Cloudflare documentation: `developers.cloudflare.com`
- OWASP Cheat Sheet Series
- Alex Edwards, *Let's Go Further*
- Google, *Software Engineering at Google*
- Google, *Building Secure & Reliable Systems*
- Martin Kleppmann and Chris Riccomini, *Designing Data-Intensive Applications*

When external behavior may have changed, verify it against current official documentation rather than relying on memory.

## Final rule

Keep this backend understandable enough that one engineer can inspect a failing request, follow it from HTTP to service to SQL, identify the invariant involved, reproduce it in a test, and fix it without first understanding a framework or distributed system.

## Git hygiene

Commit frequently on logic changes or addition (do not put chatgpt or any other author into the commit message other than me).

Keep commit messages short and easy to understand

Each logic is a commit, each feature is a separate branch.
