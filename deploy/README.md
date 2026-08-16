# Deployment references

The homelab GitOps repository owns live Kubernetes resources. This directory documents their application-facing contract; it is not an alternate source of live manifests.

| Concern | Owner |
| --- | --- |
| Deployment, Service, IngressRoute, ConfigMap, Sealed Secret, migration Job | homelab GitOps / ArgoCD |
| PostgreSQL cluster, backups, and restore configuration | CloudNativePG configuration in homelab GitOps |
| `site-admin.packetcraft.dev` Access application and identity policy | Cloudflare Access |
| public API edge rate-limit rules | Cloudflare WAF |
| API code, migrations, image, contract, and smoke command | this repository |

`backend.example.yaml` demonstrates the production contract currently expected
by `homelab-cicd-config`. Copy its documents into separate files under
`apps/portfolio/backend`; do not apply this reference file directly.

The first deployment uses the currently live `site.packetcraft.dev` origin.
Moving the public site to `packetcraft.dev` is a separate DNS, Tunnel, frontend,
and ingress cutover. See `PRE_APPLY.md` for the ordered handoff.

The administrator uses the first-level `site-admin.packetcraft.dev` hostname so
it is covered by the zone's Universal SSL certificate.

The reference demonstrates that:

- the public `site.packetcraft.dev/api/*` route reaches the backend while the frontend continues to own other public paths
- the intended admin ingress is `site-admin.packetcraft.dev/api/admin/*`; Cloudflare must protect the entire admin hostname
- Go still validates the Access assertion on every admin route, including if the public API route can reach `/api/admin/*`
- startup and liveness use `/api/healthz`; readiness uses `/api/readyz`
- probes stay outside the Access-protected admin hostname
- `DATABASE_URL` uses the existing CNPG-generated `portfolio-db-app` Secret's `uri` key, which targets the read-write Service without duplicating a database credential
- the container runs without root privileges or a writable root filesystem and has explicit resources
- an Argo CD `Sync` hook runs `/migrate` after configuration exists and before the API rollout

Configure the Access application deny-by-default, allow only the exact administrator identity, require strong authentication where the identity provider supports it, and never add a `Bypass` policy. Put its team domain and application audience in the ConfigMap; neither is a credential. Keep the administrator assertion, database URL, and visitor HMAC key out of Git and logs.

Cloudflare WAF should prioritize method/path limits for public comment, view, and like writes where the active plan supports those match fields. A coarser path rule is acceptable when it does not. Edge rules supplement the active Go limiter and PostgreSQL invariants; they do not replace them.

After ArgoCD has applied a release, run the automated smoke command described in `scripts/README.md` from a trusted runner with origin access. It verifies the normal public workflow, Go’s missing/forged-assertion rejection, authenticated moderation, and optionally the unauthenticated Access edge boundary. It does not require routine manual endpoint testing.

Do not put plaintext secrets in this directory.
