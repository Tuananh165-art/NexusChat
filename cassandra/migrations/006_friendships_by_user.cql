-- Durable friendship projection. Redis remains a hot cache for the current API.
USE "NexusChat";

CREATE TABLE IF NOT EXISTS friendships_by_user (
    user_id bigint,
    friend_id bigint,
    created_at timestamp,
    PRIMARY KEY (user_id, friend_id)
);
