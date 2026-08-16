-- Content kind is descriptive metadata. The registry row itself is the
-- allowlist boundary, so accepting future well-formed kinds does not authorize
-- arbitrary slugs.
ALTER TABLE content_items
    DROP CONSTRAINT content_items_kind,
    ADD CONSTRAINT content_items_kind CHECK (
        char_length(kind) BETWEEN 1 AND 50
        AND kind ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'
    );

-- Astro remains the source of the authored bodies. These identities authorize
-- backend-owned comments, views, likes, and aggregate statistics.
INSERT INTO content_items (slug, kind, status) VALUES
    ('k3s-cluster', 'project', 'published'),
    ('lan-drop', 'project', 'published'),
    ('obsync', 'project', 'published'),
    ('relic-overlay', 'project', 'published'),
    ('cs2105', 'review', 'published'),
    ('i7-9700k', 'review', 'published'),
    ('warframe', 'review', 'published');
