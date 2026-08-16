# Production security and reliability review

This review records the V1 controls and their ownership. It does not replace the live infrastructure review.

## Application boundaries

- Every server has explicit read-header, read, write, idle, and graceful-shutdown timeouts from validated configuration.
- PostgreSQL connection, readiness, and feature queries have separate deadlines. One shared bounded pool is closed only after HTTP draining.
- Comment JSON is capped at 16 KiB, decoded strictly, and validated with rune-aware limits. List endpoints use bounded cursor pagination.
- Panic recovery returns structured JSON without the panic value. Request cancellation propagates through service and database calls.
- Request logs use Chi route templates, never raw URL paths, and include request ID, method, status, duration, response bytes, and a coarse error category.

## Administrator boundary

- `/api/admin/*` is registered only when one route-group Cloudflare Access middleware is present.
- The verifier requires an RS256 application token, the configured issuer and audience, a non-expired token, valid optional `nbf`/`iat`, the configured administrator email, and a non-empty stable subject.
- Signing keys come from the team-domain certificates endpoint, are bounded and cached, and refresh on staleness or an unknown `kid`. Refresh errors fail closed and are logged as `access_key_refresh_failed` without the assertion.
- Moderation handlers use only the validated context principal. A real state transition and its minimal audit event commit in one transaction.
- There is no backend login, logout, password, session, refresh token, or application-issued administrator JWT.

## Public abuse controls

The current layers are strict payload validation, honeypot behavior, trusted-proxy parsing, pseudonymous visitor HMACs, rolling view deduplication, unique likes, atomic comment caps, PostgreSQL constraints, route-specific Go rate limiting, and planned Cloudflare WAF rules. Comment creation, view recording, and like-state changes have independent fixed-window allowances. Each limiter bounds retained visitor keys; its state is per pod, resets on restart, and is multiplied by the number of replicas. When the bound contains only active entries, an unseen visitor is denied until an entry expires.

Cloudflare WAF is an early filter, not a correctness boundary. Database constraints and application transactions remain authoritative. Edge thresholds should be tuned from observed legitimate traffic and checked for verified-bot impact.

## Sensitive-data policy

Application logs and audit rows must never contain Access assertions, `CF_Authorization` cookies, secrets, complete database URLs, raw IP addresses, raw user agents, complete visitor hashes, comment bodies, or raw request payloads. The approved operational identifiers are generated request IDs, route templates, a validated stable administrator subject, resource IDs, state transitions, and event categories.

## Deployment checks

The live GitOps review must confirm that only the intended Cloudflare Tunnel reaches Traefik, forwarded headers are trusted only from configured proxy CIDRs, the admin hostname is entirely Access-protected, no alternate route bypasses Go middleware, and TLS is appropriate at every hop. Run `cmd/smoke` from a trusted runner after each deployment; routine verification should not depend on manual browser, curl, or database inspection.

See [Cloudflare's JWT validation guidance](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/authorization-cookie/validating-json/) for the assertion header, audience, and rotating signing-key contract.
