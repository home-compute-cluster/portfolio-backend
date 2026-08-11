# Deployment references

The homelab GitOps repository owns live Kubernetes resources. This directory documents their application-facing contract; it is not an alternate source of live manifests.

| Concern | Owner |
| --- | --- |
| Deployment, Service, IngressRoute, ConfigMap, Sealed Secret, migration Job | homelab GitOps / ArgoCD |
| PostgreSQL cluster, backups, and restore configuration | CloudNativePG configuration in homelab GitOps |
| `admin.site.packetcraft.dev` Access application and identity policy | Cloudflare Access |
| public API edge rate-limit rules | Cloudflare WAF |
| API code, migrations, image, contract, and smoke command | this repository |

`backend.example.yaml` demonstrates the required contract:

- the public `packetcraft.dev/api/*` route reaches the backend while the frontend continues to own other public paths
- the intended admin ingress is `admin.site.packetcraft.dev/api/admin/*`; Cloudflare must protect the entire admin hostname
- Go still validates the Access assertion on every admin route, including if the public API route can reach `/api/admin/*`
- startup and liveness use `/api/healthz`; readiness uses `/api/readyz`
- probes stay outside the Access-protected admin hostname
- `DATABASE_URL` comes from a Secret and targets the CNPG read-write Service directly, without a production port-forward
- the container runs without root privileges or a writable root filesystem and has explicit resources
- a revision-specific Job runs `/migrate` once before the API rollout

Configure the Access application deny-by-default, allow only the exact administrator identity, require strong authentication where the identity provider supports it, and never add a `Bypass` policy. Put its team domain and application audience in the ConfigMap; neither is a credential. Keep the administrator assertion, database URL, and visitor HMAC key out of Git and logs.

Cloudflare WAF should prioritize method/path limits for public comment, view, and like writes where the active plan supports those match fields. A coarser path rule is acceptable when it does not. Edge rules supplement the pending Go limiter and PostgreSQL invariants; they do not replace them.

After ArgoCD has applied a release, run the automated smoke command described in `scripts/README.md` from a trusted runner with origin access. It verifies the normal public workflow, Go’s missing/forged-assertion rejection, authenticated moderation, and optionally the unauthenticated Access edge boundary. It does not require routine manual endpoint testing.

Do not put plaintext secrets in this directory.
