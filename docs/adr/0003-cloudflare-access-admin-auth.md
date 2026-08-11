# ADR 0003: Cloudflare Access owns administrator authentication

Status: accepted  
Date: 2026-08-11

## Context

The portfolio has only one or two administrators. Building password hashing,
login endpoints, session cookies, session persistence, logout, and recovery
flows would duplicate an identity boundary already available at Cloudflare.

## Decision

- Protect the entire administrator hostname with a deny-by-default Cloudflare
  Access self-hosted application and an exact-identity Allow policy.
- Independently validate `Cf-Access-Jwt-Assertion` in Go on the complete
  `/api/admin/*` route group.
- Accept only RS256 assertions with the configured issuer, application AUD,
  temporal validity, exact administrator email, and a nonempty stable subject.
- Fetch signing keys from the team `/cdn-cgi/access/certs` endpoint, cache them,
  and refresh on an unknown `kid` so key rotation requires no application build.
- Do not implement application passwords, login/logout endpoints, administrator
  sessions, refresh tokens, or application-issued JWTs for V1.

Cloudflare recommends validating the assertion header against keys from the
team certificates endpoint and selecting the matching rotated key by `kid`:
<https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/authorization-cookie/validating-json/>.
Access policies deny unmatched identities by default, while Bypass disables
Access enforcement and is therefore unsuitable for the admin surface:
<https://developers.cloudflare.com/cloudflare-one/access-controls/policies/>.

## Consequences

- Cloudflare and the configured identity provider own administrator credential
  lifecycle, MFA, login, logout, and account recovery.
- The Go verifier remains mandatory defense in depth if ingress routing is
  accidentally broadened.
- A certificates-endpoint outage rejects assertions once cached keys expire or
  when a new unknown key appears; authentication fails closed.
- Application audit records use only the validated subject identifier and never
  store assertions, Access cookies, or full administrator identity payloads.
