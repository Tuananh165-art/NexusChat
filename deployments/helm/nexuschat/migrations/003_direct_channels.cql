USE "NexusChat";

CREATE TABLE IF NOT EXISTS direct_channels_by_pair (
    user_low bigint,
    user_high bigint,
    channel_id bigint,
    created_at timestamp,
    PRIMARY KEY ((user_low, user_high))
);
