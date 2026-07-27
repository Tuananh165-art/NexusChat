USE "NexusChat";

CREATE TABLE IF NOT EXISTS websocket_tickets_by_hash (
    ticket_hash text PRIMARY KEY,
    user_id bigint,
    channel_id bigint,
    access_token text,
    expires_at timestamp
);
