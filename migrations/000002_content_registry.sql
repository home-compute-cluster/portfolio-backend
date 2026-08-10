CREATE TABLE content_items (
    slug text PRIMARY KEY,
    kind text NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT content_items_slug_format CHECK (
        char_length(slug) BETWEEN 1 AND 100
        AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'
    ),
    CONSTRAINT content_items_kind CHECK (kind IN ('post', 'project')),
    CONSTRAINT content_items_status CHECK (status IN ('draft', 'published', 'archived'))
);

-- The Astro repository remains the source of article bodies. This registry only
-- authorizes dynamic state for known, published content identities.
INSERT INTO content_items (slug, kind, status) VALUES
    ('building-a-homelab', 'post', 'published'),
    ('henz-was-probably-right', 'post', 'published'),
    ('overlays-on-non-moddable-games', 'post', 'published'),
    ('the-compiler-was-the-shell', 'post', 'published');
