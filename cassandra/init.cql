CREATE KEYSPACE IF NOT EXISTS "NexusChat"
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}
AND durable_writes = true;

USE "NexusChat";

CREATE TABLE IF NOT EXISTS channels (
    id bigint,
    user_id bigint,
    PRIMARY KEY (id, user_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id bigint,
    event int,
    channel_id bigint,
    user_id bigint,
    payload text,
    seen boolean,
    timestamp timestamp,
    edited_at timestamp,
    deleted_for_all boolean,
    deleted_by bigint,
    parent_id bigint,
    PRIMARY KEY (channel_id, id)
) WITH CLUSTERING ORDER BY (id DESC);

CREATE TABLE IF NOT EXISTS chanmsg_counters (
    channel_id bigint PRIMARY KEY,
    msgnum counter
);

CREATE TABLE IF NOT EXISTS message_reactions (
    channel_id bigint,
    message_id bigint,
    emoji text,
    user_id bigint,
    created_at timestamp,
    PRIMARY KEY ((channel_id, message_id), emoji, user_id)
);

CREATE TABLE IF NOT EXISTS pinned_messages (
    channel_id bigint,
    pinned_at timeuuid,
    message_id bigint,
    pinned_by bigint,
    PRIMARY KEY (channel_id, pinned_at)
) WITH CLUSTERING ORDER BY (pinned_at DESC);

CREATE TABLE IF NOT EXISTS channel_roles (
    channel_id bigint,
    user_id bigint,
    role text,
    assigned_at timestamp,
    PRIMARY KEY (channel_id, user_id)
);

CREATE TABLE IF NOT EXISTS notifications_by_user (
    user_id bigint,
    created_at timeuuid,
    notification_id text,
    channel_id bigint,
    type text,
    title text,
    body text,
    data text,
    is_read boolean,
    PRIMARY KEY ((user_id), created_at, notification_id)
) WITH CLUSTERING ORDER BY (created_at DESC)
AND default_time_to_live = 7776000;

CREATE TABLE IF NOT EXISTS push_subscriptions_by_user (
    user_id bigint,
    endpoint text,
    p256dh text,
    auth text,
    created_at timestamp,
    PRIMARY KEY ((user_id), endpoint)
);

CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id bigint,
    channel_id bigint,
    preference text,
    updated_at timestamp,
    PRIMARY KEY ((user_id), channel_id)
);

CREATE TABLE IF NOT EXISTS calls_by_user (
    user_id bigint,
    created_at timeuuid,
    call_id text,
    channel_id bigint,
    caller_id bigint,
    callee_id bigint,
    state text,
    media text,
    accepted_at timestamp,
    ended_at timestamp,
    end_reason text,
    PRIMARY KEY ((user_id), created_at, call_id)
) WITH CLUSTERING ORDER BY (created_at DESC)
AND default_time_to_live = 2592000;

CREATE TABLE IF NOT EXISTS calls_by_channel (
    channel_id bigint,
    created_at timeuuid,
    call_id text,
    caller_id bigint,
    callee_id bigint,
    state text,
    media text,
    accepted_at timestamp,
    ended_at timestamp,
    end_reason text,
    PRIMARY KEY ((channel_id), created_at, call_id)
) WITH CLUSTERING ORDER BY (created_at DESC)
AND default_time_to_live = 2592000;

CREATE TABLE IF NOT EXISTS processed_events (
    service text,
    event_id text,
    processed_at timestamp,
    PRIMARY KEY ((service), event_id)
) WITH default_time_to_live = 604800;

CREATE TABLE IF NOT EXISTS outbox_events (
    service text,
    bucket date,
    event_id text,
    topic text,
    payload text,
    published boolean,
    created_at timestamp,
    PRIMARY KEY ((service, bucket), created_at, event_id)
) WITH CLUSTERING ORDER BY (created_at ASC)
AND default_time_to_live = 604800;
