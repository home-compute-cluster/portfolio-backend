# Deployment references

The homelab GitOps repository owns live Kubernetes resources, including the backend Deployment, Service, IngressRoute, ConfigMap, Sealed Secret, and the CloudNativePG cluster.

`backend.example.yaml` documents the contract that those resources should implement. In particular:

- startup and liveness use `/api/healthz`
- readiness uses `/api/readyz`
- `DATABASE_URL` comes from a Secret and targets the CNPG read-write Service
- the container runs without root privileges or a writable root filesystem
- a revision-specific Job runs `/migrate` once before the API rollout

Do not put plaintext secrets in this directory.
