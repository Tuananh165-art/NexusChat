-- NexusChat migration 009: durable notification intents and retry leases.
USE "NexusChat";

CREATE TABLE IF NOT EXISTS notification_outbox_by_id (
    notification_id text PRIMARY KEY,
    user_id bigint,
    type text,
    actor_id bigint,
    payload text,
    created_at timestamp,
    status text,
    attempts int,
    next_attempt_at timestamp,
    lease_owner text,
    lease_until timestamp,
    last_error text,
    updated_at timestamp
);

CREATE TABLE IF NOT EXISTS notification_outbox_by_bucket (
    bucket text,
    status text,
    due_at timestamp,
    notification_id text,
    user_id bigint,
    type text,
    actor_id bigint,
    payload text,
    created_at timestamp,
    PRIMARY KEY ((bucket), status, due_at, notification_id)
) WITH CLUSTERING ORDER BY (status ASC, due_at ASC, notification_id ASC);
