export const EVENT_TEXT = 0;
export const EVENT_ACTION = 1;
export const EVENT_SEEN = 2;
export const EVENT_FILE = 3;
export const EVENT_EDIT = 4;
export const EVENT_DELETE = 5;
export const EVENT_REACTION = 6;
export const EVENT_PIN = 7;

export const ACCESS_TOKEN_KEY = "rc:accesstoken";

export const QUICK_EMOJIS = ["👍", "❤️", "😂", "😮", "😢", "🔥"];

export type MessageStatus = "sending" | "sent" | "delivered" | "failed";

export interface ReactionSummary {
  emoji: string;
  count: number;
  user_ids: string[];
}

export interface PinnedMessage {
  message_id: string;
  pinned_by: string;
  pinned_at: number;
}

export interface ReplyPreview {
  message_id: string;
  user_id: string;
  payload: string;
}

export interface Message {
  event: number;
  user_id: string;
  payload: string;
  message_id?: string;
  time?: string;
  seen?: boolean;
  edited_at?: number;
  deleted_for_all?: boolean;
  deleted_by?: string;
  status?: MessageStatus;
  client_id?: string;
  reactions?: ReactionSummary[];
  is_pinned?: boolean;
  parent_id?: string;
  reply_preview?: ReplyPreview;
}

export interface UserInfo {
  id: string;
  name: string;
  picture: string;
}

export interface MatchResult {
  channel_id: string;
  access_token: string;
}

export interface FilePayload {
  file_name: string;
  object_key: string;
}

export interface UploadedFile {
  name: string;
  url: string;
}

export interface PresignedUploadResult {
  url: string;
  object_key: string;
}

export interface PresignedDownloadResult {
  url: string;
}

export interface ChannelUsersResult {
  user_ids: string[];
}

export interface ChannelMessagesResult {
  messages: Message[];
}

export interface OnlineUsersResult {
  user_ids: string[];
}
