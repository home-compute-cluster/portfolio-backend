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
- static post identity synchronization
- public post comments
- comment moderation
- post view counts
- post likes
- single-admin authentication
- deployment into the existing K3s / ArgoCD / Traefik / CloudNativePG environment

The frontend remains the source of truth for authored content.

Blog posts, project descriptions, reviews, and reasonable static assets remain in the Astro/Git content workflow. The backend exists only for dynamic functionality that the static site cannot provide cleanly.

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

---

# Roadmap summary

```text
Iteration 0  Repository baseline
Iteration 1  Application walking skeleton
Iteration 2  Database migrations and integration-test foundation
Iteration 3  Static post identity synchronization
Iteration 4  Public comments
Iteration 5  Comment abuse controls
Iteration 6  Comment moderation foundation
Iteration 7  Views
Iteration 8  Likes and public stats
Iteration 9  Cloudflare Access admin authentication
Iteration 10 Authenticated moderation and audit
Iteration 11 Deployment integration
Iteration 12 Production hardening

---------------- MVP / V1 COMPLETE ----------------

Extension 1   Better observability
Extension 2   Email notifications
Extension 3   Comment replies
Extension 4   Search
Extension 5   Multiple administrators
Extension 6   Horizontal scaling
Extension 7   Richer moderation
Extension 8   Web-based content management
Extension 9   API versioning, only if needed
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
127.0.0.1:5433
    ↓ kubectl port-forward
CNPG development -rw Service
    ↓
PostgreSQL
```

Example:

```powershell
kubectl -n portfolio-dev port-forward service/portfolio-db-dev-rw 5433:5432
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

# Iteration 3 — Static post identity synchronization

## Goal

Give the backend a runtime list of valid post identities without moving authored content into PostgreSQL.

The frontend/Git repository remains the source of truth for blog content.

## Deliverables

Create a lightweight post registry.

Minimum conceptual model:

```text
slug
status
synced_at
```

Possible status:

```text
published
archived
```

Do not store the post body, MDX, Markdown, excerpt, media, or revision history in PostgreSQL as part of V1.

The registry exists only so the backend can answer:

```text
does this post slug exist and accept runtime interaction?
```

This prevents arbitrary slugs from accumulating comments, views, and likes.

## Synchronization strategy

Implement the simplest reliable approach available.

Possible approaches:

- generated manifest consumed during deployment
- explicit sync command
- build artifact containing known slugs
- migration-seeded entries during early development

Prefer a deployment-time sync that is deterministic and easy to test.

Do not build a distributed content synchronization system.

Do not turn this iteration into a CMS.

## Tests

Verify:

- unknown post slug is rejected
- known published post is accepted
- invalid slug syntax is rejected
- excessive slug length is rejected
- archived behavior is explicit
- synchronization is deterministic

## Acceptance criteria

```text
backend can determine whether a post slug is valid
comments, views, and likes can depend on this check
Astro/Git remains the authored-content source of truth
no blog body is duplicated into PostgreSQL
```

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

A real browser or curl client can:

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

# Iteration 6 — Comment moderation foundation

## Goal

Add the moderation data model and API behavior needed for later authenticated administration.

## API

Implement:

```http
GET  /api/admin/comments
POST /api/admin/comments/{id}/hide
POST /api/admin/comments/{id}/unhide
```

At this stage, these routes may exist behind a development-only guard if Iteration 9 has not yet been completed.

They must not be exposed publicly in production before Iteration 9.

Structure the route group so that final admin authentication can be applied once at:

```text
/api/admin/*
```

## Behavior

- hiding is a soft state transition
- unhiding restores visibility
- hidden comments remain queryable by admin
- public endpoints never show hidden comments
- moderation operations should be idempotent where practical
- admin comment lists are bounded and paginated

## Audit foundation

Prepare an audit-event abstraction or table if useful, but do not overbuild the final audit system before authentication exists.

Do not store full comment bodies in audit records.

## Tests

Verify:

- hide existing comment
- hide already hidden comment
- unhide existing comment
- unknown comment
- public visibility changes correctly
- moderation list is bounded/paginated
- production configuration does not expose unauthenticated moderation routes

## Acceptance criteria

Moderation semantics are correct, reversible, and ready to be secured by Iteration 9.

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

# Iteration 9 — Cloudflare Access admin authentication

## Goal

Protect administrator operations without building an application-owned password, login, JWT-issuing, or session subsystem.

Use Cloudflare Access as the primary administrator authentication boundary and validate the Access assertion again inside the Go backend before any `/api/admin/*` handler can run.

Normal visitors remain unauthenticated. Cloudflare Access applies only to the administrator surface.

## Architecture

Preferred layout:

```text
PUBLIC

packetcraft.dev
    ↓
Cloudflare
    ↓
Traefik
    ├── /*       -> Astro/nginx
    └── /api/*   -> Go public API


ADMIN

site-admin.packetcraft.dev
    ↓
Cloudflare Access
    ↓
Traefik
    ↓
Go /api/admin/*
    ↓
Access JWT validation middleware
    ↓
admin handler
```

The preferred Cloudflare Access application is the entire:

```text
site-admin.packetcraft.dev
```

rather than exposing an application-owned login page on the public site.

## Cloudflare Access policy

Configure the self-hosted Access application as deny-by-default.

Allow only the administrator identity.

Minimum policy intent:

```text
Action:   Allow
Include:  exact administrator email identity
```

Where supported by the configured identity provider, require MFA or a strong authentication method such as a passkey.

Optional device-level restrictions such as mTLS may be added later if there is a concrete requirement to restrict administration to provisioned devices.

Do not use a Cloudflare Access `Bypass` rule for the administrator surface.

## Backend validation

Cloudflare Access sends the application assertion in:

```http
Cf-Access-Jwt-Assertion: <JWT>
```

Every request reaching `/api/admin/*` must pass a single route-group middleware that validates the assertion.

The validator must verify at least:

```text
JWT signature
expected RS256 signing key
issuer (iss) == configured Cloudflare Access team domain
audience (aud) contains the configured Access application AUD tag
expiration and other relevant temporal claims
expected administrator identity, when an identity claim is used as defense in depth
```

Do not merely decode the JWT.

Do not trust the presence of the header without cryptographic verification.

## Signing-key handling

Fetch Cloudflare Access public signing keys from the team-domain Access certificates/JWKS endpoint rather than hard-coding a certificate.

The verifier must:

- cache usable signing keys
- select the correct key using the JWT `kid`
- support Cloudflare signing-key rotation
- refresh keys when necessary
- fail closed when a token cannot be validated
- use sensible network timeouts when refreshing keys

Do not fetch signing keys on every admin request.

## Configuration

Expected application configuration:

```text
CF_ACCESS_TEAM_DOMAIN
CF_ACCESS_AUD
ADMIN_EMAIL
```

Example team-domain shape:

```text
https://<team-name>.cloudflareaccess.com
```

`CF_ACCESS_TEAM_DOMAIN` and `CF_ACCESS_AUD` are configuration values, not application passwords.

Do not introduce:

```text
ADMIN_PASSWORD_HASH
admin_sessions
/api/admin/login
/api/admin/logout
application-issued admin JWTs
refresh tokens
```

for V1.

## Middleware boundary

Protect the entire route group:

```text
/api/admin/*
```

with one Access-validation middleware.

Handlers must receive a validated administrator identity from request context rather than re-reading or trusting raw Access headers independently.

Missing or invalid Access assertions must never reach an admin handler.

## Tests

Verify:

- missing `Cf-Access-Jwt-Assertion` is rejected
- invalid signature is rejected
- wrong issuer is rejected
- wrong audience is rejected
- expired token is rejected
- unexpected administrator identity is rejected when identity checking is enabled
- valid token reaches the protected handler
- signing-key rotation / changed `kid` can be handled
- JWKS/key-refresh failure fails closed
- every `/api/admin/*` route uses the middleware
- no application-owned admin login/logout endpoint exists

Use generated test signing keys or a local test JWKS server. Tests must not depend on live Cloudflare authentication.

## Acceptance criteria

```text
Cloudflare Access is the administrator authentication authority
Go independently validates the Access JWT before admin handlers run
only the configured administrator identity is authorized
no backend password/session subsystem exists
public visitors do not receive or need authentication tokens
```

---

# Iteration 10 — Authenticated moderation and audit

## Goal

Finish the administrator-facing backend around moderation and operational visibility using the validated Cloudflare Access identity from Iteration 9.

This is not a CMS or general user-account system.

## API

Required:

```http
GET  /api/admin/comments
POST /api/admin/comments/{id}/hide
POST /api/admin/comments/{id}/unhide
```

Optional if useful:

```http
GET /api/admin/stats
```

Do not add the stats endpoint unless it is actually useful to the admin frontend.

## Authentication and authorization

All endpoints in this iteration must be protected by the Iteration 9 Access middleware.

Do not perform independent authentication checks inside each handler.

The handler may consume a validated admin principal from request context for auditing, but must not trust raw `Cf-Access-*` headers directly.

If an admin route is accidentally reachable through another hostname or ingress path, the Go middleware must still reject requests without a cryptographically valid Access assertion.

## Audit events

Implement a minimal audit trail for important administrative mutations.

Expected events include:

```text
comment.hide
comment.unhide
```

Access login/logout events are owned by Cloudflare Access and do not need to be duplicated as application login/logout records.

Audit records may include:

```text
request ID
validated admin actor identifier
resource type
resource ID
previous state
new state
timestamp
```

Prefer a stable validated actor identifier. Do not persist more identity information than is operationally useful.

Do not store:

```text
Access JWTs
CF_Authorization cookies
full comment bodies
raw sensitive request payloads
```

## Pagination

Admin comment feeds must be bounded.

Prefer cursor pagination.

## Tests

Verify:

- missing/invalid Access assertion cannot reach moderation handlers
- valid Access identity can list comments
- hide works
- unhide works
- repeated moderation is safe
- audit event is recorded for mutations
- audit event identifies the validated actor without storing the Access token
- audit event contains no sensitive payload
- pagination is bounded

## Acceptance criteria

The configured administrator can reach the protected moderation API through Cloudflare Access, moderate comments safely, and leave a minimal application audit trail without the backend owning administrator credentials or sessions.

---

# Iteration 11 — Deployment integration

## Goal

Deploy the backend into the existing GitOps environment and establish the production Cloudflare trust boundaries for both public and administrator traffic.

## Kubernetes resources

Expected Kubernetes resources include:

```text
Deployment
Service
ConfigMap
Sealed Secret
IngressRoute
migration Job
```

The CNPG cluster is owned separately by the homelab infrastructure configuration.

Cloudflare Access and WAF/rate-limit configuration are Cloudflare-side controls and should have a clearly documented owner. They do not belong in PostgreSQL or application migrations.

## Public routing

Traefik continues to own the public split:

```text
Host(packetcraft.dev) && PathPrefix(/api)
    -> portfolio-backend

Host(packetcraft.dev)
    -> portfolio-frontend
```

The frontend nginx container must not proxy `/api`.

Public routes remain intentionally unauthenticated.

## Administrator routing

Create or configure:

```text
site-admin.packetcraft.dev
```

behind Cloudflare Access.

The preferred flow is:

```text
site-admin.packetcraft.dev
    -> Cloudflare Access
    -> Cloudflare Tunnel
    -> Traefik
    -> portfolio-backend /api/admin/*
```

If an admin frontend is added, the entire admin hostname should remain protected by the same Access application.

Do not create a public application login page.

Where practical, configure routing so the administrator hostname is the intended ingress path for admin operations. Regardless of routing, the Go Access-JWT middleware remains mandatory as defense in depth.

## Cloudflare Access production configuration

Configure and verify:

```text
self-hosted Access application for site-admin.packetcraft.dev
deny-by-default behavior
Allow policy for exact administrator identity
MFA/strong-auth requirement where supported
Application Audience (AUD) value
team domain
Access session duration appropriate for administration
```

The backend receives the corresponding team domain and AUD through configuration.

## Public API edge rate limiting

Cloudflare should be the first abuse-control layer for public API traffic.

Configure WAF rate-limiting rules for public API paths where the current Cloudflare plan supports the required matching fields.

Prioritize public writes:

```text
comments
likes/unlikes
views
```

and apply a broader, more generous limit to public reads when useful.

Cloudflare edge rate limiting is not the sole correctness boundary. Keep the Go-side limiter and database invariants from earlier iterations as backstops because edge rate limiting may allow some excess requests through before mitigation activates and feature availability differs by Cloudflare plan.

If the current Cloudflare plan cannot distinguish HTTP methods in a rate-limit expression, use the best path-based/coarse edge rule available and keep method-specific limits in the Go application.

Do not put Cloudflare Access in front of the normal public API solely to prevent spam.

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

Health and readiness endpoints used by Kubernetes must not accidentally be placed behind the administrator Access policy.

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

## Deployment smoke tests

Verify the public surface:

```text
health
readiness
comment create/list
view
like/unlike
public stats
```

Verify the administrator surface:

```text
admin hostname is protected by Cloudflare Access
unapproved identity is denied by Access
approved administrator identity reaches the application
missing/forged Access assertion is rejected by Go
admin comment list works
admin hide works
admin unhide works
```

Verify abuse controls:

```text
Cloudflare edge rate-limit rule is active
Go-side rate limit remains active behind it
normal frontend usage is not blocked by the configured thresholds
```

The smoke test must not depend on project editing, CMS behavior, or database-backed authored content.

## Acceptance criteria

```text
public API is reachable through packetcraft.dev/api/*
admin surface is behind Cloudflare Access
Go validates Access assertions independently
public abuse traffic is rate-limited at Cloudflare before reaching the origin where possible
application-side limits and DB constraints remain effective backstops
all core dynamic functionality works from the real frontend
```

---

# Iteration 12 — Production hardening

## Goal

Make the completed dynamic backend operationally trustworthy without expanding its product scope.

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

## Application hardening

Review:

- request timeouts
- graceful shutdown
- bounded request bodies
- structured error responses
- panic recovery
- cancellation behavior
- database query deadlines
- bounded in-memory rate-limiter state

## Cloudflare Access verifier hardening

Review and test:

```text
Cf-Access-Jwt-Assertion extraction
JWT signature verification
issuer validation
audience validation
expiration / temporal claim validation
administrator identity validation
JWKS/signing-key caching
unknown kid refresh behavior
Cloudflare signing-key rotation
network timeout while refreshing keys
fail-closed behavior
```

Do not hard-code a Cloudflare signing certificate in the binary or Kubernetes manifests.

Do not accept a JWT merely because it contains the expected email claim.

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

Security-relevant logs may include event categories such as:

```text
access_assertion_missing
access_assertion_invalid
access_key_refresh_failed
rate_limit_rejected
moderation_action
```

Do not log:

```text
Cf-Access-Jwt-Assertion values
CF_Authorization cookies
visitor HMAC keys
comment bodies
raw IP addresses
raw user agents
complete database URLs
secrets
```

Avoid logging administrator email addresses unless operationally necessary; prefer a stable validated actor identifier in application audit records.

## Database

Review:

- indexes
- query timeouts
- pool size
- migration behavior
- backup behavior
- restore procedure
- concurrency-sensitive queries

There is no V1 `admin_sessions` table to maintain or clean up.

## Public API abuse controls

Review Cloudflare and application controls together:

```text
Cloudflare WAF/rate-limiting rules
Go route-specific rate limits
honeypot behavior
request body limits
visitor HMAC behavior
view deduplication
like uniqueness
comment caps
PostgreSQL constraints
```

Tune edge thresholds using observed legitimate traffic rather than assuming the initial values are permanently correct.

Confirm that search-engine/verified-bot traffic is not unintentionally harmed by broad public-read limits.

## Network and origin security

Verify:

- origin is reached through the intended Cloudflare Tunnel path
- forwarded headers are trusted only through configured proxy boundaries
- public traffic cannot bypass intended ingress controls
- admin hostname remains Access-protected
- no alternate public route bypasses Go admin middleware
- TLS configuration is appropriate at each hop

## Observability

Ensure the system provides enough information to diagnose:

```text
request failures
database readiness failures
Cloudflare Access assertion failures
Access signing-key refresh failures
Cloudflare edge rate-limit events
application rate-limit rejections
moderation actions
```

Metrics are optional unless logs are insufficient.

## Runbooks

Document at least:

```text
database unavailable
failed migration
backend rollback
Cloudflare Tunnel unavailable
Cloudflare Access administrator lockout
Access application/AUD recreation or rotation
Access signing-key/JWKS validation failure
edge rate-limit false positive
visitor HMAC key rotation
CNPG restore
comment moderation
```

The administrator credential lifecycle is primarily owned by the configured Cloudflare Access identity provider rather than by the Go application.

Do not add CMS publishing, media-storage, revision-recovery, or content-editor runbooks to V1.

## Acceptance criteria

```text
Access authentication can fail safely without exposing admin handlers
Cloudflare signing-key rotation does not require rebuilding the backend
public abuse controls exist at both edge and application layers
Cloudflare or ingress misconfiguration cannot silently turn an admin handler into an unauthenticated handler
service can be deployed, observed, recovered, and maintained from documented procedures
```

# MVP / V1 completion definition

The backend is considered a viable V1 when Iterations 0 through 12 are complete.

The V1 product supports dynamic interaction around an otherwise static portfolio.

## Visitor

A visitor can:

```text
open a statically generated post
read visible comments
write a comment
record a view
like or unlike a post
see aggregate views and likes
```

Authored blog posts, projects, reviews, and static assets remain owned by the Astro/Git content workflow.

## Administrator

The administrator can:

```text
authenticate through Cloudflare Access
reach the private admin surface
see comments
hide comments
restore comments
```

The Go backend does not own an administrator password or login/session subsystem in V1.

The administrator does not need a backend CMS in V1.

## Operations

The system must:

```text
deploy through GitOps
use CNPG PostgreSQL
run migrations predictably
protect the admin surface with Cloudflare Access
validate Cloudflare Access JWT assertions in Go
rate-limit public API abuse at the Cloudflare edge and application layer
survive PostgreSQL outages without liveness restart loops
shut down gracefully
avoid leaking secrets or Access tokens in logs
have automated tests for important invariants
preserve the static Astro/nginx frontend architecture
```

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
- Cloudflare Access assertion failure counters
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

Add when the amount of authored content makes navigation difficult.

Because authored content remains in Astro/Git for V1, prefer a static or build-time search index first.

Possible approaches include:

```text
Astro-generated search index
client-side static search
build-time metadata index
```

Only use PostgreSQL full-text search for content that actually lives in PostgreSQL.

Do not introduce Elasticsearch/OpenSearch unless the simpler static approach is proven inadequate.

Visitor comments should not be indexed in public search by default.

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

Cloudflare Access remains the admin authentication authority across multiple API replicas.

The in-memory application rate limiter does not provide a global public-traffic limit.

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

# Extension 8 — Web-based content management

## Trigger

Add only when editing authored content through Git/MDX becomes a real maintenance burden.

This extension may introduce:

```text
web-based post editor
draft/publish workflow
database-backed authored content
revision history
media uploads
media storage
publish hooks or frontend rebuild hooks
```

Do not let this extension influence V1 database or deployment design unless it is explicitly being implemented.

Before implementation, decide deliberately whether authored content should:

1. remain Git-backed with an editor that commits content, or
2. move to PostgreSQL and be fetched/rendered from the backend.

If large media storage becomes part of the requirement, evaluate object storage at that time rather than adding it preemptively.

This extension should have its own ADR because it changes content ownership and deployment semantics.

---

# Extension 9 — API versioning

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
