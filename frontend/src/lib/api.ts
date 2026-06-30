import { ACCESS_TOKEN_KEY } from "./constants";
import type {
  UserInfo,
  ChannelUsersResult,
  ChannelMessagesResult,
  OnlineUsersResult,
  PresignedUploadResult,
  PresignedDownloadResult,
  PinnedMessage,
} from "./constants";

function getAccessToken(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem(ACCESS_TOKEN_KEY) || "";
}

function authHeaders(): HeadersInit {
  const token = getAccessToken();
  return {
    Authorization: `Bearer ${token}`,
  };
}

export async function fetchMe(): Promise<UserInfo> {
  const res = await fetch("/api/user/me", { method: "GET" });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchUser(uid: string): Promise<UserInfo> {
  const res = await fetch(`/api/user?uid=${uid}`);
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function createUser(name: string): Promise<void> {
  const res = await fetch("/api/user", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (res.status !== 201) throw new Error(res.statusText);
}

export async function updateUserProfile(name: string, picture: string): Promise<UserInfo> {
  const res = await fetch("/api/user/me", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, picture }),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchChannelUsers(): Promise<ChannelUsersResult> {
  const res = await fetch("/api/chat/users", {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchChannelMessages(pageState?: string): Promise<ChannelMessagesResult & { next_ps?: string }> {
  const params = pageState ? `?ps=${encodeURIComponent(pageState)}` : "";
  const res = await fetch(`/api/chat/channel/messages${params}`, {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchOnlineUsers(): Promise<OnlineUsersResult> {
  const res = await fetch("/api/chat/users/online", {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function deleteChannel(uid: string): Promise<void> {
  const res = await fetch(`/api/chat/channel?delby=${uid}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (res.status !== 200 && res.status !== 204) throw new Error(res.statusText);
}

export async function fetchPinnedMessages(): Promise<PinnedMessage[]> {
  const res = await fetch("/api/chat/channel/pins", {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchMyRole(uid: string): Promise<{ role: string }> {
  const res = await fetch(`/api/chat/role?uid=${uid}`, {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function searchMessages(query: string, limit = 50): Promise<ChannelMessagesResult> {
  const res = await fetch(`/api/chat/channel/search?q=${encodeURIComponent(query)}&limit=${limit}`, {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchMediaMessages(type = "all", limit = 50): Promise<ChannelMessagesResult> {
  const res = await fetch(`/api/chat/channel/media?type=${type}&limit=${limit}`, {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function fetchNotificationPrefs(uid: string): Promise<{ pref: string }> {
  const res = await fetch(`/api/chat/notification/prefs?uid=${uid}`, {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function setNotificationPrefs(uid: string, pref: string): Promise<void> {
  const res = await fetch(`/api/chat/notification/prefs?uid=${uid}&pref=${pref}`, {
    method: "PUT",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
}

export interface AIRewriteResult {
  text: string;
  provider: string;
  model?: string;
}

export async function rewriteWithAI(text: string, tone: string, locale = "Vietnamese"): Promise<AIRewriteResult> {
  const res = await fetch("/api/chat/ai/rewrite", {
    method: "POST",
    headers: {
      ...authHeaders(),
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ text, tone, locale }),
  });
  if (res.status !== 200) throw new Error(res.statusText || "AI rewrite failed");
  return res.json();
}

export type AIWorkflowType = "tasks" | "meeting_notes" | "action_items" | "checklist";

export interface AIWorkflowDraft {
  workflow_type: AIWorkflowType;
  status: string;
  preview: Record<string, unknown>;
  execution_required_approval: boolean;
}

export async function draftWorkflowWithAI(
  sourceText: string,
  workflowType: AIWorkflowType
): Promise<AIWorkflowDraft> {
  const res = await fetch("/api/ai/v1/workflows/draft", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source_text: sourceText, workflow_type: workflowType }),
  });
  if (res.status !== 200) throw new Error(res.statusText || "AI workflow failed");
  return res.json();
}

export async function getPresignedUploadUrl(
  ext: string
): Promise<PresignedUploadResult> {
  const res = await fetch(
    `/api/uploader/upload/presigned?ext=${ext}`,
    {
      method: "GET",
      headers: authHeaders(),
    }
  );
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function getPresignedDownloadUrl(
  objectKey: string
): Promise<PresignedDownloadResult> {
  const objectKeyBase64 = btoa(objectKey)
    .replace(/\+/g, "-")
    .replace(/\//g, "_");
  return { url: `/api/uploader/download/file?okb64=${encodeURIComponent(objectKeyBase64)}` };
}

export async function fetchDownloadBlobUrl(objectKey: string): Promise<string> {
  const { url } = await getPresignedDownloadUrl(objectKey);
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) throw new Error(res.statusText || "Failed to load file");
  const blob = await res.blob();
  return URL.createObjectURL(blob);
}

export async function uploadFileToPresignedUrl(
  url: string,
  file: File
): Promise<void> {
  await fetch(url, { method: "PUT", body: file });
}

export interface ChunkUploadInit {
  upload_id: string;
  object_key: string;
}

export interface ChunkPresigned {
  url: string;
}

export async function initChunkUpload(ext: string, filename: string, totalParts: number): Promise<ChunkUploadInit> {
  const res = await fetch(`/api/uploader/upload/chunk/init?ext=${ext}&filename=${filename}&total_parts=${totalParts}`, {
    method: "POST",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function getChunkPresignedUrl(objectKey: string, uploadId: string, partNumber: number): Promise<ChunkPresigned> {
  const res = await fetch(`/api/uploader/upload/chunk/presign?object_key=${objectKey}&upload_id=${uploadId}&part_number=${partNumber}`, {
    method: "GET",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
  return res.json();
}

export async function completeChunkUpload(objectKey: string, uploadId: string): Promise<void> {
  const res = await fetch(`/api/uploader/upload/chunk/complete?object_key=${objectKey}&upload_id=${uploadId}`, {
    method: "POST",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
}

export async function abortChunkUpload(objectKey: string, uploadId: string): Promise<void> {
  const res = await fetch(`/api/uploader/upload/chunk/abort?object_key=${objectKey}&upload_id=${uploadId}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (res.status !== 200) throw new Error(res.statusText);
}

const CHUNK_SIZE = 5 * 1024 * 1024;

export async function chunkedUpload(
  file: File,
  onProgress: (progress: number) => void
): Promise<string> {
  const ext = getFileExtension(file.name);
  const totalParts = Math.ceil(file.size / CHUNK_SIZE);
  if (totalParts <= 1) {
    const presigned = await getPresignedUploadUrl(ext);
    await uploadFileToPresignedUrl(presigned.url, file);
    onProgress(100);
    return presigned.object_key;
  }
  const init = await initChunkUpload(ext, file.name, totalParts);
  try {
    for (let i = 0; i < totalParts; i++) {
      const start = i * CHUNK_SIZE;
      const end = Math.min(start + CHUNK_SIZE, file.size);
      const chunk = file.slice(start, end);
      const presigned = await getChunkPresignedUrl(init.object_key, init.upload_id, i + 1);
      await fetch(presigned.url, { method: "PUT", body: chunk });
      onProgress(Math.round(((i + 1) / totalParts) * 90));
    }
    await completeChunkUpload(init.object_key, init.upload_id);
    onProgress(100);
    return init.object_key;
  } catch (err) {
    await abortChunkUpload(init.object_key, init.upload_id);
    throw err;
  }
}

export function getFileExtension(filename: string): string {
  const parts = filename.split(".");
  if (parts.length === 1 || (parts[0] === "" && parts.length === 2)) return "";
  return parts.pop()!.toLowerCase();
}

export function isImageExtension(ext: string): boolean {
  return ["jpg", "jpeg", "png", "gif", "webp"].includes(ext);
}

export function mobileCheck(): boolean {
  if (typeof navigator === "undefined") return false;
  return /android|webos|iphone|ipod|blackberry|iemobile|opera mini/i.test(
    navigator.userAgent
  );
}
