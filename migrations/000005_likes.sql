CREATE TABLE post_likes (
    post_slug text NOT NULL REFERENCES content_items (slug),
    visitor_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_slug, visitor_hash),
    CONSTRAINT post_likes_visitor_hash_length CHECK (octet_length(visitor_hash) = 32)
);
