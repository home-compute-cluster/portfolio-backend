CREATE TABLE comments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_slug text NOT NULL REFERENCES content_items (slug),
    author_name text NOT NULL,
    body text NOT NULL,
    status text NOT NULL DEFAULT 'visible',
    visitor_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    hidden_at timestamptz,
    CONSTRAINT comments_author_length CHECK (char_length(author_name) BETWEEN 1 AND 80),
    CONSTRAINT comments_author_trimmed CHECK (author_name = btrim(author_name)),
    CONSTRAINT comments_body_length CHECK (char_length(body) BETWEEN 1 AND 2000),
    CONSTRAINT comments_body_trimmed CHECK (body = btrim(body)),
    CONSTRAINT comments_status CHECK (status IN ('visible', 'hidden')),
    CONSTRAINT comments_visitor_hash_length CHECK (octet_length(visitor_hash) = 32),
    CONSTRAINT comments_hidden_state CHECK (
        (status = 'visible' AND hidden_at IS NULL)
        OR (status = 'hidden' AND hidden_at IS NOT NULL)
    )
);

CREATE INDEX comments_visible_post_page
    ON comments (post_slug, id DESC)
    WHERE status = 'visible';

CREATE INDEX comments_visitor_activity
    ON comments (visitor_hash, created_at DESC);
