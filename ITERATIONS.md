# ITERATIONS.md

## Purpose

This file defines the implementation roadmap for the `packetcraft.dev` backend.

It complements `AGENTS.md`:

- `AGENTS.md` defines **how** the code should be designed and maintained.
- `ITERATIONS.md` defines **what order** the backend should be built in and what must be true before moving forward.

The intended working model is incremental delivery. Each iteration should leave the repository in a buildable, testable, deployable state.

Do not attempt to implement the entire roadmap in one change.

---

# Product target

The initial viable backend must support:

- application health and readiness
- PostgreSQL connectivity
- database migrations
- public post comments
- comment moderation
- post view counts
- post likes
- single-admin authentication
- admin-managed project content
- deployment into the existing K3s / ArgoCD / Traefik / CloudNativePG environment

The frontend is the only intended API consumer.

The API therefore starts without explicit `/v1` versioning.

Use:

```text
/api/...
```

Do not introduce:

```text
/api/v1/...
```

solely for future-proofing.

Prefer backward-compatible API evolution. Introduce a new API version only when a genuine incompatible contract must coexist with the previous contract.

---

# Iteration rules

Every iteration must follow these rules.

1. Inspect the current repository before making changes.
2. Do not implement later iterations unless required to complete the current iteration correctly.
3. Keep every iteration independently buildable.
4. Add tests with the implementation.
5. Run relevant tests before marking an iteration complete.
6. Add migrations whenever persistence changes.
7. Update configuration examples when new configuration is introduced.
8. Update OpenAPI documentation when API behavior changes.
9. Do not weaken security or correctness to make an iteration easier.
10. Avoid speculative abstractions for future iterations.
11. Prefer one coherent vertical slice over many incomplete layers.
12. Stop and document blockers when an external dependency genuinely prevents completion.
13. Do not silently change architecture established in `AGENTS.md`.
14. Significant architecture changes require an ADR.
15. Automate acceptance wherever technically practical; do not make routine browser, curl, or database inspection a required human step.

Examples that describe what a browser or curl client can observe define external behavior. They must be demonstrated by automated HTTP or integration tests unless the behavior cannot reasonably be automated and the exception is documented.

---

# Roadmap summary

```text
Iteration 0  Repository baseline
Iteration 1  Application walking skeleton
Iteration 2  Database migrations and integration-test foundation
Iteration 3  Content identity
Iteration 4  Public comments
Iteration 5  Comment abuse controls
Iteration 6  Comment moderation
Iteration 7  Views
Iteration 8  Likes and public stats
Iteration 9  Admin authentication
Iteration 10 Admin project management
Iteration 11 Deployment integration
Iteration 12 Production hardening

---------------- MVP / V1 COMPLETE ----------------

Extension 1   Observability improvements
Extension 2   Email notifications
Extension 3   Comment replies
Extension 4   Search
Extension 5   Multi-admin support
Extension 6   Horizontal scaling
Extension 7   Richer moderation
Extension 8   API versioning, only if needed
```

---

# Iteration 0 — Repository baseline

## Goal

Establish a clean Go repository that Codex and humans can work in safely.

## Deliverables

Create or verify:

```text
go.mod
go.sum
AGENTS.md
ITERATIONS.md
README.md
.env.example
.gitignore
Makefile
Dockerfile
cmd/api/
internal/
migrations/
deploy/
scripts/
```

Add initial dependencies only when required.

Expected core dependencies:

```text
github.com/go-chi/chi/v5
github.com/jackc/pgx/v5
```

Do not add ORM, DI, authentication, migration, metrics, or validation libraries unless a current iteration requires them.

## README minimum

Document:

- purpose of the service
- local development prerequisites
- how to configure `DATABASE_URL`
- how to run the API
- how to run tests
- how local PostgreSQL access works
- expected Kubernetes environment at a high level

## Acceptance criteria

```text
go mod tidy succeeds
go test ./... succeeds
go build ./cmd/api succeeds
repository structure follows AGENTS.md
no secrets are committed
```

---

# Iteration 1 — Application walking skeleton

## Goal

Produce the smallest real backend process that can run locally and inside Kubernetes.

## Deliverables

Implement:

- typed configuration
- `log/slog` logger
- PostgreSQL pool construction
- Chi router
- `/api/healthz`
- `/api/readyz`
- HTTP server timeouts
- graceful shutdown
- basic unit tests

## Required behavior

### Health

```http
GET /api/healthz
```

Returns:

```json
{"status":"ok"}
```

with HTTP `200`.

It must not query PostgreSQL.

### Readiness

```http
GET /api/readyz
```

Returns:

```json
{"status":"ready"}
```

with HTTP `200` when PostgreSQL is reachable.

Returns:

```json
{"status":"unavailable"}
```

with HTTP `503` when PostgreSQL cannot be reached.

The PostgreSQL error must be logged internally but never sent to the client.

## Development database workflow

Support the following development model:

```text
local Go API
    ↓
127.0.0.1:15432
    ↓ kubectl port-forward
CNPG development -rw Service
    ↓
PostgreSQL
```

Example:

```powershell
kubectl -n portfolio-dev port-forward service/portfolio-db-dev-rw 15432:5432
```

The backend must not contain special logic for the port-forward.

It only consumes `DATABASE_URL`.

## Tests

At minimum:

- health returns 200
- health does not depend on database
- readiness returns 200 for successful pinger
- readiness returns 503 for failed pinger
- malformed configuration fails startup validation
- router returns 404 for unknown route
- panic recovery middleware behaves correctly

## Acceptance criteria

```text
go run ./cmd/api starts successfully
/api/healthz returns 200
/api/readyz returns 200 with PostgreSQL
/api/readyz returns 503 without PostgreSQL
SIGTERM shuts the server down cleanly
go test ./... passes
go test -race ./... passes
go build ./cmd/api passes
```

---

# Iteration 2 — Migrations and database test foundation

## Goal

Make schema evolution reproducible and database-specific behavior testable.

## Deliverables

Implement:

- SQL migration mechanism
- migration command or migration job entrypoint
- initial migration directory convention
- PostgreSQL integration-test harness
- CI PostgreSQL service where practical

Preferred structure:

```text
cmd/
├── api/
└── migrate/

migrations/
├── 000001_*.sql
├── 000002_*.sql
└── ...
```

Use one migration execution per deployment.

Do not make every API pod race to run migrations.

## Integration testing

Integration tests must run against PostgreSQL.

Do not use SQLite as a substitute.

Test infrastructure should make it easy for future iterations to verify:

- constraints
- transactions
- indexes
- concurrency
- migrations

## Acceptance criteria

```text
empty database can migrate to latest schema
migration failure returns non-zero exit status
integration tests can connect to PostgreSQL
migrations are deterministic
migration state is versioned
CI can run database-dependent tests
```

---

# Iteration 3 — Content identity

## Goal

Give the backend an authoritative list of valid content identities without moving static article bodies into PostgreSQL.

## Deliverables

Create a lightweight content registry.

Example conceptual model:

```text
slug
kind
status
created_at
updated_at
```

Possible kinds:

```text
post
project
```

Possible status:

```text
draft
published
archived
```

The static Astro frontend remains responsible for blog content.

The backend uses the registry only to prevent arbitrary slugs from accumulating comments, views, and likes.

## Synchronization strategy

Implement the simplest reliable approach available.

Possible approaches:

- generated manifest consumed during deployment
- explicit sync command
- admin registration
- migration-seeded entries during early development

Do not build a distributed content synchronization system.

## Tests

Verify:

- unknown post slug is rejected
- known published post is accepted
- invalid slug syntax is rejected
- excessive slug length is rejected
- draft/archived behavior is explicit

## Acceptance criteria

```text
backend can determine whether a post slug is valid
anonymous write endpoints can depend on this check later
no static article body is duplicated into PostgreSQL
```

---

# Iteration 4 — Public comments

## Goal

Deliver the first complete user-facing vertical slice.

## API

Implement:

```http
GET  /api/posts/{slug}/comments
POST /api/posts/{slug}/comments
```

## Comment model

Minimum fields:

```text
id
post_slug
author_name
body
status
visitor_hash
created_at
hidden_at
```

Initial states:

```text
visible
hidden
```

Comments publish immediately.

## Validation

Enforce:

- valid existing post
- author name required
- author name length limit
- body required
- comment body character limit
- request body byte limit
- whitespace-only rejection
- valid JSON
- bounded response sizes

Use rune-aware character counting when limits are specified in characters.

## Public rendering contract

Comments are plain text.

Do not support raw HTML in comments.

## Pagination

Do not return an unbounded list.

Prefer cursor pagination.

Example shape:

```text
GET /api/posts/{slug}/comments?before_id=123&limit=25
```

Enforce a server-side maximum limit.

## Database behavior

Use appropriate indexes for:

- public visible comment listing
- visitor/post lookup

Prefer a partial index for visible comments.

## Tests

Test:

- successful creation
- successful listing
- unknown post
- malformed JSON
- oversized request
- empty author
- whitespace-only body
- Unicode length boundaries
- pagination
- hidden comments excluded from public results

## Acceptance criteria

Automated HTTP tests demonstrate that a client can:

```text
create comment
receive created comment
list comment publicly
paginate comments
```

All tests pass.

---

# Iteration 5 — Comment abuse controls

## Goal

Make optimistic comment publishing sufficiently safe for public exposure.

## Deliverables

Implement:

- visitor identity
- honeypot field
- per-IP or per-visitor rate limiting
- per-post comment cap
- request-size enforcement
- trusted client-IP extraction

## Visitor identity

Use a pseudonymous HMAC-derived identifier.

Concept:

```text
HMAC-SHA-256(
    VISITOR_HMAC_KEY,
    normalized-client-IP + separator + normalized-user-agent
)
```

Do not store raw IP or raw user-agent values solely for visitor identity.

## Proxy trust

Traffic is expected to flow:

```text
Cloudflare
    ↓
Cloudflare Tunnel
    ↓
Traefik
    ↓
backend
```

Do not blindly trust arbitrary forwarded headers.

Client-IP extraction must be centralized.

## Comment cap correctness

Do not implement this as an unsafe independent:

```text
COUNT
INSERT
```

sequence.

The limit check and insertion must form one correctness boundary using appropriate PostgreSQL locking or transaction semantics.

## Rate limiter

An in-memory limiter is acceptable initially.

Its memory usage must be bounded.

Document that:

- rate-limit state resets on restart
- limits are per-pod
- scaling replicas changes effective global limits

## Tests

Add tests for:

- honeypot rejection
- rate limit
- rate limit recovery
- spoofed forwarded headers
- IPv4 client parsing
- IPv6 client parsing
- malformed forwarding headers
- visitor HMAC determinism
- comment cap
- concurrent attempts around the comment cap

## Acceptance criteria

```text
simple spam bursts are throttled
honeypot submissions are rejected
visitor identity is pseudonymous
comment cap cannot trivially race above its intended bound
proxy-header trust rules are explicit and tested
```

---

# Iteration 6 — Comment moderation

## Goal

Allow the administrator to remove inappropriate comments from public display without destroying data.

## API

Implement:

```http
GET  /api/admin/comments
POST /api/admin/comments/{id}/hide
POST /api/admin/comments/{id}/unhide
```

At this iteration, temporary development-only protection may be used only if admin authentication has not yet been implemented, but the code must be structured so the final admin middleware is inserted at the `/api/admin` route group.

Do not expose these routes publicly in production without Iteration 9.

## Behavior

- hiding is a soft state transition
- unhiding restores visibility
- hidden comments remain queryable by admin
- public endpoints never show hidden comments
- moderation operations should be idempotent where practical

## Audit foundation

Introduce an admin audit-event abstraction or table if needed for later auth and project management.

Do not store full comment bodies in the audit event.

## Tests

Verify:

- hide existing comment
- hide already hidden comment
- unhide existing comment
- unknown comment
- public visibility changes correctly
- moderation list is bounded/paginated

## Acceptance criteria

Moderation works correctly and does not permanently destroy comment data.

---

# Iteration 7 — Views

## Goal

Add approximate, abuse-resistant post view counts.

## API

Implement:

```http
POST /api/posts/{slug}/view
```

## Deduplication

Choose and document one precise model:

### Option A — calendar day

One counted view per visitor/post/database-calendar-day.

### Option B — rolling window

One counted view per visitor/post over a configured rolling time duration.

Prefer rolling-window behavior if the configuration is expressed as hours.

Do not claim a `DATE` key is a rolling 24-hour deduplication mechanism.

## Response

If processing completes before the response, use:

```text
204 No Content
```

Do not return `202 Accepted` merely because the frontend does not await the request.

Do not spawn untracked goroutines from the handler.

## Atomicity

Deduplication and count increment must behave atomically.

Concurrent duplicate requests must not inflate the count unexpectedly.

## Tests

Verify:

- first view counts
- duplicate view does not count
- view after expiry counts
- different visitor counts
- unknown post rejected
- concurrent duplicate requests

## Acceptance criteria

Public traffic can record views without obvious refresh-spam inflation.

---

# Iteration 8 — Likes and public stats

## Goal

Deliver user likes and a public post statistics endpoint.

## API

Implement:

```http
PUT    /api/posts/{slug}/like
DELETE /api/posts/{slug}/like
GET    /api/posts/{slug}/stats
```

Do not implement a toggle endpoint.

## Behavior

`PUT` means:

```text
ensure this visitor has liked this post
```

`DELETE` means:

```text
ensure this visitor has not liked this post
```

Both should be idempotent.

## Persistence

Use a unique key such as:

```text
(post_slug, visitor_hash)
```

If cached like totals are stored, modification of the unique like row and total must be atomic.

At expected project scale, deriving likes using `COUNT(*)` is acceptable if measured performance is adequate.

## Stats response

Example:

```json
{
  "views": 123,
  "likes": 17
}
```

Do not expose internal visitor identifiers.

## Tests

Verify:

- first like
- repeated like
- unlike
- repeated unlike
- concurrent likes
- stats correctness
- unknown post

## Acceptance criteria

The website can display stable likes and views for each valid post.

---

# Iteration 9 — Admin authentication

## Goal

Secure all administrator endpoints using a simple single-admin session model.

## Scope

There is one administrator.

Do not implement:

- OAuth
- social login
- JWT refresh tokens
- registration
- password reset workflows
- user roles

unless requirements change.

## API

Implement:

```http
POST /api/admin/login
POST /api/admin/logout
```

Protect:

```text
/api/admin/*
```

using route-group middleware.

## Credential storage

Use an Argon2id password hash provided through secret configuration.

Do not store the single administrator password in PostgreSQL.

## Sessions

Use:

- cryptographically random opaque token
- server-side session record
- hash of token persisted in PostgreSQL
- expiration time
- secure cookie

Cookie properties:

```text
HttpOnly
Secure
SameSite=Strict
Path=/
Domain omitted
```

Prefer a `__Host-` cookie name.

## Cross-site protection

Add an additional protection for admin writes, such as:

- `Origin` validation
- Fetch Metadata validation

Do not treat `SameSite` as the only defense.

## Login abuse control

Apply a strict login-attempt rate limit.

Do not reveal whether a specific password-processing stage failed.

## Audit

Record appropriate events:

```text
login success
login failure category if safe
logout
comment hide
comment unhide
```

Do not audit passwords, session tokens, or full sensitive request bodies.

## Tests

Verify:

- correct password
- incorrect password
- session cookie flags
- missing session
- invalid session
- expired session
- logout
- repeated logout
- admin routes inaccessible without auth
- cross-site request rejection
- login rate limiting

## Acceptance criteria

No `/api/admin/*` operational endpoint is reachable without a valid session.

---

# Iteration 10 — Admin project management

## Goal

Allow project content to be created and maintained without rebuilding backend code.

## API

Implement:

```http
GET    /api/projects
GET    /api/projects/{slug}

POST   /api/admin/projects
PUT    /api/admin/projects/{slug}
DELETE /api/admin/projects/{slug}
```

If irreversible deletion is not required, prefer archive semantics.

## Project model

Suggested fields:

```text
slug
title
summary
body
status
version
created_at
updated_at
```

Suggested status:

```text
draft
published
archived
```

## Optimistic concurrency

Use a version column.

Updates must identify the expected version.

A stale update should return:

```text
409 Conflict
```

rather than silently overwrite newer content.

## Markdown

Treat Markdown rendering as a security boundary.

Prefer disabling raw embedded HTML.

If HTML is permitted, sanitize output deliberately.

## Public behavior

Only published projects should appear publicly.

Draft and archived content must remain available to authenticated administration as appropriate.

## Tests

Verify:

- create
- update
- stale update conflict
- publish
- archive/delete behavior
- public listing
- public single-project lookup
- draft hidden publicly
- unknown slug
- auth requirements

## Acceptance criteria

Project data can be managed from the admin API safely and appears correctly in the public API.

---

# Iteration 11 — Deployment integration

## Goal

Deploy the backend into the existing GitOps environment.

## Kubernetes resources

Expected resources include:

```text
Deployment
Service
ConfigMap
Sealed Secret
IngressRoute
migration Job
```

The CNPG cluster is owned separately by the homelab infrastructure configuration.

## Routing

Traefik owns the split:

```text
Host(packetcraft.dev) && PathPrefix(/api)
    → portfolio-backend

Host(packetcraft.dev)
    → portfolio-frontend
```

The frontend nginx container must not proxy `/api`.

## Database

The deployed backend connects directly to the CNPG read-write Service.

No `kubectl port-forward` is used in production.

## Probes

Configure:

```text
startupProbe   -> /api/healthz
livenessProbe  -> /api/healthz
readinessProbe -> /api/readyz
```

Database failure affects readiness only.

## Container expectations

Where practical:

- run as non-root
- read-only root filesystem
- no shell requirement at runtime
- explicit resource requests/limits
- graceful SIGTERM handling

## Deployment workflow

Expected model:

```text
source
  ↓
CI
  ↓
container image
  ↓
GHCR
  ↓
GitOps image/tag update
  ↓
ArgoCD
  ↓
migration
  ↓
backend rollout
```

## Tests

At minimum run an automated deployment smoke test:

```text
health
readiness
public project read
comment create/list
view
like
admin login
admin moderation
```

## Acceptance criteria

The backend is reachable through:

```text
https://packetcraft.dev/api/...
```

and all core functionality works from the real frontend.

---

# Iteration 12 — Production hardening

## Goal

Make the completed MVP operationally trustworthy.

## CI

Target checks:

```text
gofmt
go vet ./...
staticcheck ./...
go test ./...
go test -race ./...
govulncheck ./...
integration tests
go build ./cmd/api
docker build
container vulnerability scan
```

## Logging

Ensure structured request logging includes:

```text
request_id
method
route template
status
duration
response bytes
error category
```

Do not log:

```text
passwords
cookies
raw session tokens
comment bodies
project bodies
raw IP addresses
raw user agents
complete database URL
HMAC keys
```

## Database

Review:

- indexes
- query timeouts
- pool size
- migration behavior
- backup behavior
- recovery procedure

## Security

Review:

- TLS
- proxy trust
- forwarded-header handling
- admin authentication
- cookie flags
- cross-site protections
- request-size limits
- rate limits
- dependency vulnerabilities

## Runbooks

Document at least:

```text
database unavailable
failed migration
backend rollback
admin credential rotation
visitor HMAC key rotation
CNPG restore
comment moderation
```

## Acceptance criteria

The service can be deployed, observed, recovered, and maintained without relying on undocumented knowledge.

---

# MVP / V1 completion definition

The backend is considered a viable V1 when Iterations 0 through 12 are complete.

The V1 product must support the following end-to-end experience.

## Visitor

A visitor can:

```text
open a post
read visible comments
write a comment
record a view
like or unlike a post
see aggregate views and likes
view published projects
```

## Administrator

The administrator can:

```text
log in
log out
see comments
hide comments
restore comments
create projects
edit projects
publish projects
archive/delete projects according to the chosen policy
```

## Operations

The system must:

```text
deploy through GitOps
use CNPG PostgreSQL
run migrations predictably
survive PostgreSQL outages without liveness restart loops
shut down gracefully
avoid leaking secrets in logs
have automated tests for important invariants
```

---

# Post-MVP extensions

These are intentionally not part of the initial viable product.

Do not implement them merely because they are listed.

Each extension should begin with a concrete requirement and, when architectural impact is significant, an ADR.

---

# Extension 1 — Better observability

## Trigger

Add when debugging production behavior from logs alone becomes difficult.

Possible work:

- Prometheus metrics
- request latency histograms
- database query duration metrics
- rate-limit counters
- moderation counters
- admin login failure counters
- dashboards
- alerting

Avoid tracing until there is a real debugging need.

A single-process backend rarely benefits from distributed tracing early.

---

# Extension 2 — Email notifications

## Trigger

Add when the administrator wants notification of new comments or other events.

Do not send email synchronously inside comment creation.

Preferred design:

```text
application transaction
    ├── insert comment
    └── insert outbox event

background worker
    ↓
claim event
    ↓
send email
    ↓
mark event complete
```

Use a transactional outbox to prevent lost notifications caused by process crashes.

The worker may remain part of the same codebase and image initially.

Do not create a separate microservice unless operational needs justify it.

---

# Extension 3 — Comment replies

## Trigger

Add when threaded discussion is actually desired.

Possible schema extension:

```text
parent_comment_id
```

Constraints should prevent invalid cross-post parents.

Decide explicitly whether replies to hidden comments remain visible.

Consider maximum nesting depth.

Do not create arbitrary recursive behavior without pagination and moderation rules.

---

# Extension 4 — Search

## Trigger

Add when the amount of project or blog content makes navigation difficult.

Start with PostgreSQL full-text search.

Do not introduce Elasticsearch/OpenSearch until PostgreSQL search is proven inadequate.

Potential searchable content:

```text
project title
project summary
project body
post metadata
```

Visitor comments should probably not be indexed in public search by default.

---

# Extension 5 — Multiple administrators

## Trigger

Add only when another human needs administrative access.

Then introduce:

```text
admins table
admin IDs
role model if actually required
actor IDs in audit records
credential lifecycle
password reset/rotation process
```

Do not migrate to OAuth solely because there is more than one administrator.

Evaluate the authentication requirements at that time.

---

# Extension 6 — Horizontal scaling

## Trigger

Add when one backend pod is insufficient for availability or load.

Before scaling replicas, review:

```text
rate limiting
session storage
database connection budget
migration execution
background workers
view deduplication
pod disruption behavior
graceful rollout behavior
```

Database-backed sessions already support multiple API replicas.

The in-memory rate limiter does not provide a global limit.

Possible later solutions include:

- edge rate limiting at Cloudflare
- Traefik-level limiting
- Redis-backed shared limiting

Prefer using an existing infrastructure layer before adding a new datastore solely for rate limiting.

---

# Extension 7 — Richer moderation

## Trigger

Add only if comment volume or abuse justifies it.

Possible additions:

```text
moderation reason
temporary bans
visitor blocks
spam scoring
admin notes
bulk moderation
comment reporting
captcha
```

Do not add CAPTCHA preemptively.

Current default philosophy is:

```text
prevent obvious spam
publish immediately
moderate reactively
```

Revisit only when observed abuse demonstrates the need.

---

# Extension 8 — API versioning

## Trigger

Introduce explicit API versions only when:

1. the API gains external consumers, or
2. a breaking contract must coexist with the existing contract for a meaningful period.

Current API remains:

```text
/api/...
```

Do not rename everything to `/api/v1` simply because versioning might be useful someday.

Prefer additive changes such as:

```text
new optional response fields
new endpoints
new query parameters with safe defaults
```

If a genuinely incompatible API is later required, introduce the new version deliberately.

Example:

```text
/api/...
/api/v2/...
```

A prior `/v1` namespace is not required in order to introduce a future `/v2`.

Document:

- reason for versioning
- old API support period
- migration path
- removal criteria

before adding the new version.

---

# Optional future extensions

Other possible features should remain outside the implementation plan until a concrete requirement appears.

Examples:

```text
comment reactions
RSS-related dynamic metadata
webhooks
admin dashboards
scheduled publishing
project image management
full comment deletion
content revision history
analytics
backup API
public API access
API keys
mobile client support
```

Do not treat this list as a backlog that must eventually be implemented.

---

# Iteration completion checklist

Before marking any iteration complete, verify:

```text
[ ] requirement implemented
[ ] repository builds
[ ] relevant unit tests added
[ ] integration tests added where persistence behavior matters
[ ] gofmt applied
[ ] go vet passes
[ ] go test ./... passes
[ ] go test -race ./... passes when applicable
[ ] migration added when schema changed
[ ] .env.example updated when config changed
[ ] OpenAPI updated when API changed
[ ] README/runbook updated when operational behavior changed
[ ] no secrets added
[ ] no unrelated refactoring
[ ] acceptance criteria demonstrated
```

---

# Agent handoff format

When an agent finishes an iteration, its final summary should state:

```text
Iteration:
What was implemented:
Files changed:
Migrations added:
API changes:
Configuration changes:
Tests added:
Commands run:
Known limitations:
Next recommended iteration:
```

Do not claim an iteration is complete if its acceptance criteria have not been met.

If only part of an iteration was completed, state that clearly.

---

# Guiding principle

Each iteration should create a small amount of finished, production-oriented functionality.

Prefer:

```text
one complete feature
```

over:

```text
five half-built future abstractions
```

The roadmap is successful when the backend stays understandable, deployable, and easy to change throughout the entire build rather than becoming "architecturally complete" before it becomes useful.
