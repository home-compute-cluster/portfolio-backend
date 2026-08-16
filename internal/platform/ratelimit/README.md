# Pod-local rate limiting

`AssignmentLimiter` implements a concurrency-safe fixed window for each
pseudonymous visitor hash. Comments, view recordings, and like-state changes use
separate limiter instances and therefore do not consume one another's allowance.

The implementation deliberately has no cleanup goroutine. When a new visitor
would exceed `maxKeys`, the limiter synchronously removes expired windows. If all
retained entries are still active, the unseen visitor is denied rather than an
active entry being evicted.

Operational limits:

- state belongs to one pod and resets when that pod restarts
- multiple replicas multiply the effective allowance
- `maxKeys` bounds each limiter independently, so three configured limiters may
  retain up to three times that number of visitor keys per pod
- Cloudflare edge controls supplement this limiter but do not replace it

Run its focused concurrency and behavior suite with:

```powershell
go test -race -tags assignment ./internal/platform/ratelimit
```
