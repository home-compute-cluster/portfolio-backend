# Rate-limiting assignment

Implement `AssignmentLimiter` as an in-memory, pod-local limiter. You may choose
a fixed window, sliding window, or token bucket, provided the acceptance behavior
remains clear and documented.

Requirements:

- safe under concurrent requests
- separate allowance per pseudonymous visitor hash
- reject requests above the configured allowance
- permit traffic again as time advances
- retain no more than `maxKeys` visitor entries
- do not start an untracked cleanup goroutine
- document that restarts reset state and replicas multiply the effective limit

Run the assignment suite with:

```powershell
go test -race -tags assignment ./internal/platform/ratelimit
```

The template is intentionally not constructed in `internal/app` until these
tests pass. Once complete, pass it to `NewCommentHandler` and add the rate values
to typed configuration.
