CREATE TABLE post_view_visitors (
    post_slug text NOT NULL REFERENCES content_items (slug),
    visitor_hash bytea NOT NULL,
    last_counted_at timestamptz NOT NULL,
    PRIMARY KEY (post_slug, visitor_hash),
    CONSTRAINT post_view_visitors_hash_length CHECK (octet_length(visitor_hash) = 32)
);

CREATE TABLE post_stats (
    post_slug text PRIMARY KEY REFERENCES content_items (slug),
    view_count bigint NOT NULL DEFAULT 0,
    CONSTRAINT post_stats_view_count_nonnegative CHECK (view_count >= 0)
);
