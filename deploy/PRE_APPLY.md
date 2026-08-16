# Production pre-apply checklist

This is the ordered handoff from the application repository to
`BarneyLaw/homelab-cicd-config`. The only live deployment command at the end is
the Argo CD `Application` bootstrap. Do not directly apply the workload
Kustomization because the migration ordering is owned by Argo CD hooks and sync
waves.

## Verified starting state

The following were confirmed against the live cluster on 2026-08-16:

- the production namespace is `portfolio`
- `portfolio-db` is healthy and CNPG exposes `portfolio-db-app` with a `uri` key
- Argo CD, CloudNativePG, Traefik, and Sealed Secrets are healthy
- the Traefik pod network is within `10.42.0.0/16`
- the four published frontend post slugs match migration `000002`
- `site.packetcraft.dev` resolves and serves the current frontend
- `packetcraft.dev` and `admin.site.packetcraft.dev` do not yet resolve

Recheck these facts if deployment happens after an infrastructure change.

## 1. Supply release credentials

Create a fine-grained token that can write repository contents only in
`BarneyLaw/homelab-cicd-config`. Add it to the backend repository as the Actions
secret `CONFIG_REPO_TOKEN`.

The release job publishes:

```text
ghcr.io/home-compute-cluster/portfolio-backend:<seven-character-commit-sha>
```

Make the GHCR package public before applying the workload. If the package must
remain private, add a Sealed Secret containing a registry credential and add
`imagePullSecrets` to both the Deployment and migration Job instead.

## 2. Add the GitOps workload

Create this structure in `homelab-cicd-config`:

```text
apps/portfolio/backend/
  configmap.yaml
  deployment.yaml
  ingressroute.yaml
  kustomization.yaml
  migration-job.yaml
  sealed-secret.yaml
  service.yaml
argocd/
  portfolio-backend.yaml
```

Split the documents in `backend.example.yaml` into the correspondingly named
files. The Kustomization should be:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: portfolio
resources:
  - configmap.yaml
  - sealed-secret.yaml
  - migration-job.yaml
  - deployment.yaml
  - service.yaml
  - ingressroute.yaml
images:
  - name: ghcr.io/home-compute-cluster/portfolio-backend
    newName: ghcr.io/home-compute-cluster/portfolio-backend
    newTag: replace-before-apply
```

The Argo CD application should be:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: portfolio-backend
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/BarneyLaw/homelab-cicd-config.git
    targetRevision: main
    path: apps/portfolio/backend
  destination:
    server: https://kubernetes.default.svc
    namespace: portfolio
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

Commit these files before merging the backend release workflow, but do not
apply `argocd/portfolio-backend.yaml` yet. The first successful backend release
will replace `replace-before-apply` with its immutable image tag.

## 3. Create the application secret

Generate a new stable visitor HMAC key and seal it for the exact Secret name and
namespace. Do not reuse the database password and do not commit the plaintext.
The installed controller is currently `sealed-secrets-controller` in
`kube-system` at version `0.27.1`.

One PowerShell workflow is:

```powershell
$visitorKey = [Convert]::ToBase64String(
    [Security.Cryptography.RandomNumberGenerator]::GetBytes(48)
)

kubectl create secret generic portfolio-backend `
    --namespace portfolio `
    --from-literal="VISITOR_HMAC_KEY=$visitorKey" `
    --dry-run=client `
    -o json |
    go run github.com/bitnami-labs/sealed-secrets/cmd/kubeseal@v0.27.1 `
        --controller-name sealed-secrets-controller `
        --controller-namespace kube-system `
        --format yaml |
    Set-Content apps/portfolio/backend/sealed-secret.yaml

Remove-Variable visitorKey
```

Review only the encrypted SealedSecret output. Its template must create
`portfolio-backend` in `portfolio` with the `VISITOR_HMAC_KEY` key.

## 4. Finish Cloudflare and proxy trust

Before the API starts, replace the ConfigMap placeholders with:

- the HTTPS Cloudflare Access team domain
- the Access application's audience (`AUD`)
- the exact allowed administrator email
- `TRUSTED_PROXY_CIDRS=10.42.0.0/16` for the Traefik-to-backend hop

Create `admin.site.packetcraft.dev` on the same Tunnel and protect the entire
hostname with one deny-by-default Access application. Allow only the intended
administrator identity and require strong authentication. Never add a Bypass
policy.

The native `cloudflared` connector runs on the K3s node `deus` and sends its
origin requests through the node-local K3s ServiceLB listener on port 80. The
live ServiceLB rules masquerade that traffic before forwarding it to Traefik,
so Traefik sees the ServiceLB pod as its direct peer rather than the node's
`192.168.1.250` address. Trust the stable pod CIDR assigned to `deus`, not the
current ephemeral ServiceLB pod IP:

```yaml
# apps/traefik/helmchartconfig.yaml, merged into the existing valuesContent
ports:
  web:
    forwardedHeaders:
      trustedIPs:
        - "10.42.0.0/24"
```

Do not enable `forwardedHeaders.insecure`. The API separately trusts the full
K3s pod network (`10.42.0.0/16`) so its right-to-left forwarded-chain walk can
pass both the Traefik pod and the ServiceLB hop. Cloudflare public edge ranges
do not belong in either list because the edge does not connect directly to
Traefik.

The initial public backend route remains:

```text
Host(`site.packetcraft.dev`) && PathPrefix(`/api`)
```

The frontend continues to own every non-`/api` path. The later apex-domain
cutover must update Cloudflare DNS/Tunnel routing, the frontend build origin,
and both frontend and backend IngressRoutes together.

## 5. Publish and promote the first image

After the GitOps workload directory and `CONFIG_REPO_TOKEN` exist, merge the
backend release branch to `main`. CI must complete all Go, PostgreSQL, race,
container, and vulnerability checks before the release job:

1. publishes the SHA-tagged image to GHCR
2. runs Kustomize against `apps/portfolio/backend`
3. commits the immutable tag to `homelab-cicd-config/main`

Do not apply while the Kustomization still contains `replace-before-apply` or
an image tag that is absent from GHCR.

## 6. Validate without changing the cluster

From the GitOps repository, run:

```powershell
rg -n "replace-|git-sha" apps/portfolio/backend argocd/portfolio-backend.yaml
kubectl kustomize apps/portfolio/backend
kubectl apply --dry-run=server -k apps/portfolio/backend
kubectl apply --dry-run=server -f argocd/portfolio-backend.yaml
kubectl diff -f argocd/portfolio-backend.yaml
```

The placeholder search must return no matches. Both server-side dry runs must
succeed. Review the Kustomize output to confirm that the Deployment and
migration Job use the same immutable image and that no plaintext Secret exists.

## 7. Apply boundary

When every preceding item is complete, the deployment is ready to begin with:

```powershell
kubectl apply -f argocd/portfolio-backend.yaml
```

That command creates the Argo CD application. Argo CD then applies configuration
at wave `-2`, runs the migration hook at wave `-1`, and rolls out the API at wave
`0`. Post-deployment acceptance is the automated `cmd/smoke` workflow described
in `scripts/README.md`, not manual endpoint probing.
