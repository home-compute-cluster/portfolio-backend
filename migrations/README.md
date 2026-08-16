# Migrations

The application owns versioned, forward-only PostgreSQL migrations in this directory.

Content identities are synchronized by adding a forward-only migration whenever
the Astro site publishes, archives, or renames a blog, project, review, or other
content item. Authored bodies remain in the frontend repository; this registry
only authorizes backend-owned dynamic state. Kind is constrained descriptive
metadata, while the explicit registry row and its publication state form the
authorization boundary. Editing an already-applied seed migration is
intentionally rejected by the migration checksum guard.

## Convention

Files must be consecutive and use six-digit versions:

```text
000001_baseline.sql
000002_add_something.sql
```

Never edit or remove an applied migration. The runner stores each filename and SHA-256 checksum in `schema_migrations` and rejects changed or missing history. Add a new migration for every schema change.

All migrations execute in one PostgreSQL transaction guarded by a transaction-scoped advisory lock. A failure rolls back the entire run, including its migration-state changes. Consequently, migration SQL must be valid inside a transaction; operations such as `CREATE INDEX CONCURRENTLY` require a deliberate future runner extension.

## Execution

The API never runs migrations. The separate entrypoint is:

```text
/migrate -dir /migrations
```

Local binaries can be built with `make build`. In deployment, GitOps runs one revision-specific migration Job before rolling out the API pods.

## Automated verification

`make test-integration` runs migrations against real PostgreSQL using `TEST_DATABASE_URL`. Tests create unique temporary schemas, verify migration state and rollback behavior, and remove those schemas automatically. CI provides its own PostgreSQL service and runs these tests on every push and pull request.
