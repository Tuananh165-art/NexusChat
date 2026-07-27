USE "NexusChat";

CREATE TABLE IF NOT EXISTS channel_metadata_by_id (
    channel_id bigint PRIMARY KEY,
    kind text,
    created_at timestamp
);

CREATE TABLE IF NOT EXISTS message_read_state_by_user (
    channel_id bigint,
    user_id bigint,
    last_read_message_id bigint,
    updated_at timestamp,
    PRIMARY KEY ((channel_id), user_id)
);

CREATE TABLE IF NOT EXISTS notifications_by_user (
    user_id bigint,
    created_at timestamp,
    notification_id text,
    type text,
    actor_id bigint,
    payload text,
    read boolean,
    PRIMARY KEY ((user_id), created_at, notification_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
