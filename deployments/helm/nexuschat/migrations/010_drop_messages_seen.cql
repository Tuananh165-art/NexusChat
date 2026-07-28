-- NexusChat migration 010: remove legacy per-message read flags.
-- Durable read state is stored in message_read_state_by_user.
USE "NexusChat";

ALTER TABLE messages DROP seen;
