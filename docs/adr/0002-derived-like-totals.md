# ADR 0002: Derive like totals from unique rows

Status: accepted  
Date: 2026-08-10

## Context

Likes must be idempotent for each `(post_slug, visitor_hash)` pair and public
statistics must return a stable like total. Maintaining both a unique like row
and a cached counter would require every like and unlike to update two pieces of
state atomically.

## Decision

- Store one `post_likes` row per post and pseudonymous visitor, enforced by a
  composite primary key.
- Implement `PUT` with `INSERT ... ON CONFLICT DO NOTHING` and `DELETE` with a
  matching predicate. Each command reports whether persistent state changed,
  while HTTP returns the same `204` response for repeated desired-state calls.
- Derive the public like total with `count(*)`. Read that count and the existing
  cached view total in one PostgreSQL statement.
- Reconsider a cached like counter only after measurements demonstrate that the
  indexed count is a real bottleneck.

PostgreSQL documents that a multicolumn primary key supplies uniqueness and a
B-tree index: <https://www.postgresql.org/docs/current/ddl-constraints.html>.
Its `ON CONFLICT DO NOTHING` behavior provides the atomic insert-or-no-op
operation: <https://www.postgresql.org/docs/current/sql-insert.html>.
`count(*)` returns a `bigint` row count:
<https://www.postgresql.org/docs/current/functions-aggregate.html>.

## Consequences

- There is no cached like counter that can drift from its source rows.
- Concurrent likes from the same visitor resolve through the primary key.
- Like reads cost an indexed row count. This is acceptable for the site's
  expected scale and keeps write correctness straightforward.
