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
