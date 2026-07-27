-- Durable friend-request state. Redis remains a compatibility cache for legacy clients.
USE "NexusChat";

CREATE TABLE IF NOT EXISTS friend_requests_by_recipient (
    recipient_id bigint,
    sender_id bigint,
    status text,
    created_at timestamp,
    updated_at timestamp,
    PRIMARY KEY ((recipient_id), sender_id)
);