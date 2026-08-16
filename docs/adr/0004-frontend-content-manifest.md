# ADR 0004: Synchronize frontend content with a deployment manifest

Status: accepted

## Context

Astro owns blogs, project readings, reviews, and future static collections.
PostgreSQL must recognize those identities before it accepts dynamic
interactions, but adding content rows through schema migrations makes routine
publishing unnecessarily manual. Creating a row on the first anonymous comment
would let public traffic invent identities and defeat the registry allowlist.

The frontend also owns the `comments` switch. The backend must enforce that
policy for direct API callers rather than relying only on whether Astro renders
a form.

## Decision

Frontend CI generates a versioned, complete JSON snapshot from Astro's loaded
collections. GitOps mounts it into a one-shot `/sync-content` Job after schema
migrations and before application/frontend rollout.

The synchronizer:

- strictly validates size, schema version, full-snapshot mode, source, revision, identity formats,
  publication states, required comment policy, and global slug uniqueness
- serializes jobs with a transaction-scoped PostgreSQL advisory lock
- uses `INSERT ... ON CONFLICT DO UPDATE` for idempotent identity/policy changes
- archives source-owned rows omitted from the complete snapshot
- never deletes comments, views, likes, or registry rows
- rejects attempts by one manifest source to take another source's slug
- applies the snapshot in one transaction

Comment reads require a published item with comments enabled. Comment creation
rechecks that policy with a `FOR SHARE` row lock in the same transaction as its
limit check and insert, preventing a policy update from racing through the
write boundary.

## Consequences

Publishing a reading no longer requires a SQL migration. Schema changes remain
forward-only migrations, while content lifecycle changes become deterministic
frontend build artifacts.

The manifest is authoritative and therefore must be complete. Delta mode,
empty, malformed, and duplicate snapshots fail closed. The backend cannot
distinguish an intended removal from a generator that omitted a collection, so
frontend CI must test collection coverage. Removing an item archives it;
restoring it preserves prior interaction history.

Frontend and registry promotion need explicit deployment ordering. A shared
Argo CD Application can use waves; separate Applications need an automated
two-stage promotion that waits for registry synchronization before publishing
the frontend image.

This introduces no public registration endpoint, background service, queue, or
additional database. The sync command uses the existing backend image and CNPG
credentials.

## Rejected alternatives

- **A migration per reading:** safe but couples content publishing to backend
  schema history and requires routine manual coordination.
- **Register on first interaction:** lets anonymous callers create arbitrary
  registry identities and unbounded state.
- **Fetch the frontend repository at API startup:** adds network availability,
  credentials, and nondeterministic startup behavior to every replica.
- **Store article bodies in PostgreSQL:** duplicates Astro's responsibility and
  contradicts the static-site architecture.

## References

- [PostgreSQL explicit and advisory locks](https://www.postgresql.org/docs/current/explicit-locking.html)
- [PostgreSQL `INSERT ... ON CONFLICT`](https://www.postgresql.org/docs/current/sql-insert.html)
- [Argo CD sync phases and waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/)
