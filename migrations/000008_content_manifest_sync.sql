-- The Astro manifest is authoritative for frontend-owned identity, publication,
-- and comment policy. Existing seeded rows all came from that repository.
ALTER TABLE content_items
    ADD COLUMN comments_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN managed_by text NOT NULL DEFAULT 'portfolio-site',
    ADD CONSTRAINT content_items_managed_by_format CHECK (
        char_length(managed_by) BETWEEN 1 AND 50
        AND managed_by ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'
    );

COMMENT ON COLUMN content_items.comments_enabled IS
    'Whether published content accepts and exposes visitor comments.';

COMMENT ON COLUMN content_items.managed_by IS
    'Manifest source allowed to update or archive this identity.';
