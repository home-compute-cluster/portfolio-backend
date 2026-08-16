# Frontend content registry synchronization

The Astro repository is the source of truth for content bodies and publication
rules. PostgreSQL stores only the identity and policy needed for comments,
views, and likes. Anonymous API traffic can never create registry rows.

## Manifest contract

Frontend CI must produce one complete JSON snapshot matching
[`api/content-registry.schema.json`](../api/content-registry.schema.json):

```json
{
  "schema_version": 1,
  "mode": "full",
  "source": "portfolio-site",
  "revision": "0123456789abcdef0123456789abcdef01234567",
  "items": [
    {
      "slug": "building-a-homelab",
      "kind": "blog",
      "status": "published",
      "comments_enabled": true
    }
  ]
}
```

`revision` is the 7-to-64-character lowercase hexadecimal frontend commit ID.
Slugs are globally unique across collections because the stable interaction API
contains only `{slug}`, not `{collection}/{slug}`. A duplicate therefore fails
the deployment instead of silently attaching interactions to the wrong page.

`mode` must be `full`; delta manifests are never accepted. The snapshot must
include every content entry owned by `portfolio-site`, not
only changed or comment-enabled entries. Include drafts as `draft`; use the same
publication predicate that decides whether Astro emits or links a page. An item
omitted from a valid snapshot becomes `archived`. Archiving never deletes its
comments, views, or likes, and restoring the item in a later snapshot makes that
history available again.

`comments_enabled` is required for every item. The safe frontend schema is:

```ts
comments: z.boolean().default(false)
```

Only content whose frontmatter or JSON explicitly sets `comments: true` should
emit `comments_enabled: true`. Views, likes, and stats require only `published`;
comment listing and creation require both `published` and `comments_enabled`.

## Frontend build change

Generate the snapshot from Astro's loaded collections rather than parsing MDX
or JSON a second time. The generator should:

1. load `blog`, `projects`, and `reviews` with `getCollection`
2. map each entry ID to `slug`
3. map the collection to a descriptive lower-kebab `kind`
4. reuse the site's publication rule to set `status`
5. map `entry.data.comments === true` to `comments_enabled`
6. sort by slug for deterministic Git diffs
7. set `revision` from `GITHUB_SHA`
8. reject duplicate slugs before writing `dist/content-registry.json`

A prerendered Astro JSON endpoint is one straightforward way to run this inside
Astro's content environment. CI may copy its generated file out of `dist`; the
registry contains no secret. Alternatively, use an Astro integration hook that
writes the same artifact. In either case, make registry generation and schema
validation part of `npm run build` or the existing full CI gate.

Do not derive the registry by crawling generated HTML. The collection loader
already has the authoritative IDs, frontmatter defaults, and publication data.

## GitOps change

Store the generated file at:

```text
apps/portfolio/backend/content-registry.json
```

Have Kustomize create the ConfigMap so content changes receive a content hash:

```yaml
configMapGenerator:
  - name: portfolio-content-registry
    files:
      - content-registry.json=content-registry.json
generatorOptions:
  annotations:
    argocd.argoproj.io/sync-wave: "-2"
```

Add the content-sync Job from
[`deploy/backend.example.yaml`](../deploy/backend.example.yaml). Its ordering is:

```text
wave -2  configuration and generated registry ConfigMap
wave -1  schema migration Job
wave  0  content registry sync Job
wave  1  backend Deployment
```

The Job runs `/sync-content -manifest /content/content-registry.json` using the
same immutable backend image and CNPG `portfolio-db-app` Secret as migrations.
It validates a bounded, strict full-manifest contract and applies the snapshot
in one transaction under an advisory lock. Malformed, empty, duplicate, and
cross-source snapshots fail before the API rollout. The backend cannot infer
that a nonempty generated list accidentally missed a collection, so frontend CI
must test that the generator enumerates every configured collection.

## Automated release ordering

The initial rollout must deploy backend migration `000008` and the new command
before frontend CI starts publishing manifests.

For later content releases, registry sync must finish before the corresponding
frontend image becomes public. If frontend and backend resources share one Argo
CD Application, enforce this with sync waves. If they remain separate
Applications, use an automated two-stage frontend promotion:

1. commit the generated registry to GitOps
2. wait for the backend Argo Application and content-sync Job to succeed
3. commit/promote the frontend image

The wait belongs in CI through the Argo CD API/CLI or an appropriately scoped
deployment runner; it is not a manual endpoint test. A failed registry sync must
stop frontend promotion.

Never register a slug when an anonymous visitor first comments. That would turn
the public write endpoint into an unbounded content-creation API and remove the
registry's allowlist guarantee.
