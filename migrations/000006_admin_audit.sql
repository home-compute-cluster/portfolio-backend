CREATE TABLE admin_audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type text NOT NULL,
    actor_id text NOT NULL,
    request_id text,
    resource_type text NOT NULL,
    resource_id bigint NOT NULL,
    previous_state text NOT NULL,
    new_state text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT admin_audit_event_type CHECK (event_type IN ('comment.hide', 'comment.unhide')),
    CONSTRAINT admin_audit_actor_length CHECK (char_length(actor_id) BETWEEN 1 AND 255),
    CONSTRAINT admin_audit_request_id_length CHECK (request_id IS NULL OR char_length(request_id) BETWEEN 1 AND 128),
    CONSTRAINT admin_audit_resource CHECK (resource_type = 'comment' AND resource_id > 0),
    CONSTRAINT admin_audit_previous_state CHECK (previous_state IN ('visible', 'hidden')),
    CONSTRAINT admin_audit_new_state CHECK (new_state IN ('visible', 'hidden')),
    CONSTRAINT admin_audit_state_changed CHECK (previous_state <> new_state)
);

CREATE INDEX admin_audit_events_created_page
    ON admin_audit_events (id DESC);
