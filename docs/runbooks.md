# Operations runbooks

These procedures are for incidents and recovery, where targeted operator inspection is necessary. Routine acceptance is automated by CI and `cmd/smoke`. Never paste secrets, database URLs, cookies, or Access assertions into tickets, chat, or logs.

## Database unavailable

1. Confirm `/api/healthz` remains healthy and `/api/readyz` is failing; do not change liveness to query PostgreSQL or restart-loop healthy API processes.
2. Inspect the CloudNativePG `Cluster`, pods, events, and operator logs in the GitOps-owned namespace. Check the read-write Service endpoints and the backend readiness logs.
3. Verify that the deployed Secret references the intended database and that pool limits do not exceed the cluster connection budget. Do not print the connection string.
4. Restore connectivity or failover through CloudNativePG. If data recovery is required, use the CNPG restore procedure below.
5. Wait for readiness to recover, then run the automated deployment smoke command.

## Failed migration

1. Stop the rollout while leaving the previous compatible API revision serving. Inspect the revision-specific migration Job logs.
2. The runner applies all pending migrations in one transaction, so a SQL failure rolls back that run. Do not edit an already-applied migration or modify production schema manually.
3. Correct the fault with a new forward migration or a code fix, add a regression integration test, and produce a new image.
4. Let GitOps create a new revision-specific Job. Continue the API rollout only after it succeeds and run deployment smoke automation.

## Backend rollback

1. Revert the GitOps image reference to the last known-good immutable tag; do not use an imperative cluster change that ArgoCD will overwrite.
2. Confirm the older binary is compatible with the current schema. Forward-only migrations are not automatically reversed; use expand-and-contract for changes spanning releases.
3. Let ArgoCD reconcile, confirm probes, and run the smoke command. If compatibility is uncertain, keep the new release stopped and roll forward with a fix.

## Cloudflare Tunnel unavailable

1. Determine whether this is a down tunnel (`1033`) or a connected tunnel that cannot reach its origin (`502`).
2. Check connector health and `cloudflared` logs, then validate its Kubernetes Service target, protocol, port, network policy, and origin TLS name.
3. Restore or add a connector replica through the infrastructure repository. Do not expose a direct public origin as a shortcut.
4. Run deployment smoke automation through the public and admin hostnames after recovery. Follow [Cloudflare's Tunnel troubleshooting guide](https://developers.cloudflare.com/cloudflare-one/troubleshooting/tunnel/) for status-specific diagnosis.

## Cloudflare Access administrator lockout

1. Keep the Go middleware fail closed. Do not add a `Bypass` policy or temporary unauthenticated route.
2. From a separately secured Cloudflare account, verify the identity provider, exact-email Allow policy, MFA requirement, application domain, and session settings.
3. Restore the intended identity or use the second pre-authorized administrator when configured. The application has no password reset or local session to repair.
4. Run the admin portion of `cmd/smoke` with a fresh short-lived assertion from a trusted runner.

## Access application or AUD recreation

1. Treat deletion/recreation as a configuration rotation: Cloudflare documents that an application's AUD changes when the application is recreated.
2. Obtain the new AUD and confirm the team domain from the Cloudflare dashboard without copying an assertion.
3. Update `CF_ACCESS_AUD` and, only if changed, `CF_ACCESS_TEAM_DOMAIN` in GitOps, deploy, and expect old application assertions to fail.
4. Run admin smoke automation. Keep the previous deployment available only if its old Access application still exists and remains protected.

## Access signing-key or JWKS validation failure

1. Use `access_key_refresh_failed` logs and request IDs to distinguish network/key retrieval failure from invalid assertions. Never log the token.
2. Check DNS, egress, TLS trust, clock synchronization, the configured team domain, and the official `/cdn-cgi/access/certs` endpoint.
3. Do not hard-code a certificate, extend token expiry, skip signature checks, or accept email claims without verification.
4. Once key retrieval recovers, the cache refreshes without a rebuild. Confirm with the automated admin smoke checks. Cloudflare documents regular signing-key rotation in its [JWT validation guide](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/authorization-cookie/validating-json/).

## Edge rate-limit false positive

1. Correlate the Cloudflare event with route, method, time window, and request ID where available; do not collect raw visitor identity in application logs.
2. Narrow or raise the offending WAF rule using observed legitimate traffic, and check verified-bot traffic before applying broad public-read limits.
3. Keep public writes protected and retain the Go limiter/database backstops. Do not put Cloudflare Access in front of the normal public API.
4. Run deployment smoke automation at the normal threshold and a separate controlled load test for limit behavior.

## Visitor HMAC key rotation

1. Generate a new high-entropy key in the secret-management workflow and update only the Sealed Secret; never commit or log the plaintext.
2. Roll API pods once. There is no dual-key period because this identity is abuse prevention, not authentication.
3. Expect existing visitors to receive new pseudonymous identities: view deduplication and like state may restart, while old hashes remain opaque database values.
4. Run normal tests and deployment smoke automation. Do not bulk-delete interaction data unless a separate reviewed retention change requires it.

## CloudNativePG restore

1. Identify the required recovery point and verify that the base backup and WAL archive cover it.
2. Create a new CNPG cluster through GitOps using recovery bootstrap; do not restore in place over the damaged cluster.
3. Validate migration history, content identities, comment visibility, reaction totals, and application queries against the recovered cluster before switching the backend Secret/Service reference.
4. Reconcile the backend, confirm probes, and run deployment smoke automation before retiring the old cluster. CloudNativePG's [recovery documentation](https://cloudnative-pg.io/docs/1.26/recovery/) describes the new-cluster bootstrap model and PITR requirements.

## Comment moderation

1. Authenticate through the Access-protected admin hostname and use the explicit hide or unhide operation; there is no public delete or toggle endpoint.
2. A successful real transition creates `comment.hide` or `comment.unhide` audit metadata in the same transaction. Repeating the desired state is safe and does not add another audit row.
3. If unhide returns a conflict, the post is at its visible-comment cap. Hide another comment or leave the target hidden; do not bypass the invariant in SQL.
4. Use request ID, stable actor ID, resource ID, and state transition for diagnosis. Never copy comment bodies into operational records.
