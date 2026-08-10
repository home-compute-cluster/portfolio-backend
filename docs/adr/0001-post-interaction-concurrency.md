# ADR 0001: PostgreSQL boundaries for post interactions

Status: accepted  
Date: 2026-08-10

## Context

Comments have a per-post visible limit, moderation changes whether a row counts
toward that limit, and views must increment at most once per visitor in a rolling
window. Separate read-then-write statements without serialization would violate
those invariants under concurrent requests.

## Decision

- Keep frontend-owned article bodies out of PostgreSQL. `content_items` is only
  the authoritative identity and publication-state registry for backend dynamic
  state.
- Serialize comment creation and visibility transitions per post with a
  transaction-level advisory lock. The visible count and mutation execute in the
  same transaction, while unrelated posts remain independent.
- Store comment visibility explicitly as `visible` or `hidden`, reinforce the
  state/timestamp relationship with a database check constraint, and index only
  visible rows for public pagination.
- Store one view-deduplication row per `(post_slug, visitor_hash)`. A single
  data-modifying CTE uses `INSERT ... ON CONFLICT DO UPDATE ... WHERE` to advance
  the rolling timestamp only after the cutoff, and increments `post_stats` only
  when that CTE returns a row.
- Pass application time into view persistence so rolling-window behavior is
  deterministic in automated tests.

PostgreSQL documents transaction-scoped advisory locks as automatically held
until the transaction ends: <https://www.postgresql.org/docs/current/functions-admin.html#FUNCTIONS-ADVISORY-LOCKS>.
It also specifies that rows skipped by an `ON CONFLICT DO UPDATE ... WHERE`
condition are not returned, which is the basis of the atomic view decision:
<https://www.postgresql.org/docs/current/sql-insert.html>.
The visible-comments index follows PostgreSQL's partial-index model:
<https://www.postgresql.org/docs/current/indexes-partial.html>.

## Consequences

- Correctness does not depend on one API replica or in-process mutexes.
- Heavy activity on one post serializes that post's comment mutations, which is
  acceptable at current scale.
- `post_stats` is derived cached state and must only be changed in the same SQL
  statement as the deduplication decision.
- Content publication changes require a new forward-only application migration;
  already-applied migrations remain immutable.
