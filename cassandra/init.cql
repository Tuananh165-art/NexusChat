CREATE KEYSPACE IF NOT EXISTS "NexusChat"
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}
AND durable_writes = true;

USE "NexusChat";

CREATE TABLE IF NOT EXISTS schema_migrations (
    migration_id text PRIMARY KEY,
    applied_at timestamp
);

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

CREATE TABLE IF NOT EXISTS chat_rooms_by_id (
    channel_id bigint PRIMARY KEY,
    name text,
    owner_id bigint,
    invite_code text,
    member_count int,
    archived boolean,
    created_at timestamp,
    updated_at timestamp
);

CREATE TABLE IF NOT EXISTS chat_rooms_by_user (
    user_id bigint,
    updated_at timestamp,
    channel_id bigint,
    name text,
    owner_id bigint,
    invite_code text,
    member_count int,
    role text,
    PRIMARY KEY (user_id, updated_at, channel_id)
) WITH CLUSTERING ORDER BY (updated_at DESC, channel_id DESC);

CREATE TABLE IF NOT EXISTS chat_room_invites (
    invite_code text PRIMARY KEY,
    channel_id bigint,
    created_by bigint,
    created_at timestamp,
    expires_at timestamp
);

CREATE TABLE IF NOT EXISTS safety_rules (
    rule_id text PRIMARY KEY,
    name text,
    pattern text,
    score int,
    enabled boolean,
    created_at timestamp,
    updated_at timestamp
);

CREATE TABLE IF NOT EXISTS safety_reports_by_id (
    report_id text PRIMARY KEY,
    reporter_id bigint,
    target_user_id bigint,
    channel_id bigint,
    message_id bigint,
    reason text,
    details text,
    status text,
    created_at timestamp,
    updated_at timestamp
);

CREATE TABLE IF NOT EXISTS safety_reports_by_status (
    status text,
    created_at timestamp,
    report_id text,
    reporter_id bigint,
    target_user_id bigint,
    channel_id bigint,
    message_id bigint,
    reason text,
    PRIMARY KEY ((status), created_at, report_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS safety_blocks_by_user (
    user_id bigint,
    blocked_user_id bigint,
    created_at timestamp,
    PRIMARY KEY (user_id, blocked_user_id)
);

CREATE TABLE IF NOT EXISTS safety_decisions_by_channel (
    channel_id bigint,
    created_at timestamp,
    decision_id text,
    message_id bigint,
    user_id bigint,
    score int,
    action text,
    reason text,
    PRIMARY KEY ((channel_id), created_at, decision_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS safety_decisions_by_user (
    user_id bigint,
    created_at timestamp,
    decision_id text,
    channel_id bigint,
    message_id bigint,
    score int,
    action text,
    reason text,
    PRIMARY KEY ((user_id), created_at, decision_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS safety_appeals_by_report (
    report_id text,
    created_at timestamp,
    appeal_id text,
    user_id bigint,
    reason text,
    status text,
    PRIMARY KEY ((report_id), created_at, appeal_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS user_risk_snapshots (
    user_id bigint PRIMARY KEY,
    score int,
    reason text,
    updated_at timestamp
);

CREATE TABLE IF NOT EXISTS discovery_profiles_by_user (
    user_id bigint PRIMARY KEY,
    languages text,
    interests text,
    avoid_topics text,
    conversation_goal text,
    enabled boolean,
    updated_at timestamp
);

CREATE TABLE IF NOT EXISTS discovery_feedback_by_user (
    user_id bigint,
    created_at timestamp,
    feedback_id text,
    peer_id bigint,
    channel_id bigint,
    rating int,
    tags text,
    comment text,
    updated_at timestamp,
    PRIMARY KEY ((user_id), created_at, feedback_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS discovery_matches_by_user (
    user_id bigint,
    matched_at timestamp,
    match_id text,
    peer_id bigint,
    channel_id bigint,
    score double,
    PRIMARY KEY ((user_id), matched_at, match_id)
) WITH CLUSTERING ORDER BY (matched_at DESC);

CREATE TABLE IF NOT EXISTS workspace_items_by_channel (
    channel_id bigint,
    updated_at timestamp,
    item_id text,
    owner_id bigint,
    kind text,
    title text,
    content text,
    status text,
    priority text,
    assignees text,
    tags set<text>,
    message_id bigint,
    due_at timestamp,
    created_at timestamp,
    PRIMARY KEY ((channel_id), updated_at, item_id)
) WITH CLUSTERING ORDER BY (updated_at DESC);

CREATE TABLE IF NOT EXISTS workspace_items_by_id (
    item_id text PRIMARY KEY,
    channel_id bigint,
    updated_at timestamp,
    owner_id bigint,
    kind text,
    title text,
    content text,
    status text,
    priority text,
    assignees text,
    tags set<text>,
    message_id bigint,
    due_at timestamp,
    created_at timestamp
);

CREATE TABLE IF NOT EXISTS workspace_bookmarks_by_user (
    user_id bigint,
    created_at timestamp,
    item_id text,
    channel_id bigint,
    title text,
    tags set<text>,
    PRIMARY KEY ((user_id), created_at, item_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS workspace_collections_by_user (
    user_id bigint,
    collection_id text,
    name text,
    created_at timestamp,
    updated_at timestamp,
    PRIMARY KEY (user_id, collection_id)
);

CREATE TABLE IF NOT EXISTS workspace_checklists_by_item (
    item_id text,
    checklist_id text,
    text text,
    done boolean,
    created_at timestamp,
    updated_at timestamp,
    PRIMARY KEY (item_id, checklist_id)
);

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

CREATE TABLE IF NOT EXISTS direct_channels_by_pair (
    user_low bigint,
    user_high bigint,
    channel_id bigint,
    created_at timestamp,
    PRIMARY KEY ((user_low, user_high))
);

CREATE TABLE IF NOT EXISTS channel_metadata_by_id (channel_id bigint PRIMARY KEY, kind text, created_at timestamp);
CREATE TABLE IF NOT EXISTS direct_channels_by_id (channel_id bigint PRIMARY KEY, user_low bigint, user_high bigint, created_at timestamp);
CREATE TABLE IF NOT EXISTS websocket_tickets_by_hash (ticket_hash text PRIMARY KEY, user_id bigint, channel_id bigint, access_token text, expires_at timestamp);
CREATE TABLE IF NOT EXISTS message_read_state_by_user (channel_id bigint, user_id bigint, last_read_message_id bigint, updated_at timestamp, PRIMARY KEY ((channel_id), user_id));
CREATE TABLE IF NOT EXISTS notifications_by_user (user_id bigint, created_at timestamp, notification_id text, type text, actor_id bigint, payload text, read boolean, PRIMARY KEY ((user_id), created_at, notification_id)) WITH CLUSTERING ORDER BY (created_at DESC);
CREATE TABLE IF NOT EXISTS friendships_by_user (user_id bigint, friend_id bigint, created_at timestamp, PRIMARY KEY (user_id, friend_id));
CREATE TABLE IF NOT EXISTS friend_requests_by_recipient (recipient_id bigint, sender_id bigint, status text, created_at timestamp, updated_at timestamp, PRIMARY KEY ((recipient_id), sender_id));
