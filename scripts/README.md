# Scripts

Keep small development and operational helpers here when a repeated workflow justifies one. The local database port-forward is developer-managed, so the API itself contains no port-forward logic.

## Deployment smoke test

`go run ./cmd/smoke` provides an automated post-deployment check. It exercises health, readiness, stats, view, like/unlike, comment create/list, missing and forged Access assertions, authenticated admin list, and hide/unhide. Supplying the optional admin edge URL also verifies that Cloudflare Access does not return a successful response to an unauthenticated request.

The command intentionally creates one comment, proves both moderation transitions, and leaves that generated comment hidden. Use an existing published post and remove old smoke records through a future retention process only if their small audit footprint becomes operationally relevant.

Supply the short-lived Access assertion through a restricted temporary file so it is not exposed in the command line or logs:

```text
go run ./cmd/smoke \
  -public-url https://packetcraft.dev \
  -admin-origin-url http://portfolio-backend.portfolio-dev.svc.cluster.local \
  -admin-edge-url https://admin.site.packetcraft.dev \
  -post-slug building-a-homelab \
  -access-assertion-file /run/secrets/cf-access-assertion
```

Run it from trusted CI or a Kubernetes Job that can reach the internal Service. The assertion file and URLs are runtime inputs and must not be committed. A successful run exits zero; any contract or trust-boundary failure exits non-zero without printing response bodies or the assertion.
