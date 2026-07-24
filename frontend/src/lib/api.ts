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

function authHeaders(userId?: string): HeadersInit {
  const token = getAccessToken();
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
  };
  if (userId) headers["X-User-Id"] = userId;
  return headers;
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

export interface SafetyReport {
  id: string;
  target_user_id: string;
  message_id?: string;
  reason: string;
  details?: string;
  status: string;
  created_at: string;
}

export async function createSafetyReport(input: { target_user_id: string; message_id?: string; reason: string; details?: string }): Promise<SafetyReport> {
  const res = await fetch("/api/safety/reports", {
    method: "POST",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export async function fetchSafetyBlocks(): Promise<{ user_id: string; created_at: string }[]> {
  const res = await fetch("/api/safety/blocks", { headers: authHeaders() });
  if (!res.ok) throw new Error(res.statusText);
  return (await res.json()).blocks || [];
}

export async function blockSafetyUser(userId: string): Promise<void> {
  const res = await fetch("/api/safety/blocks", {
    method: "POST",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify({ user_id: userId }),
  });
  if (!res.ok) throw new Error(res.statusText);
}

export async function updateDiscoveryProfile(input: Record<string, unknown>): Promise<void> {
  const res = await fetch("/api/discovery/profile", {
    method: "PUT",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(res.statusText);
}

export async function fetchDiscoveryProfile(): Promise<Record<string, unknown>> {
  const res = await fetch("/api/discovery/profile", { headers: authHeaders() });
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export interface WorkspaceItem {
  id: string;
  kind: "task" | "note" | "bookmark";
  title: string;
  content?: string;
  status: "todo" | "in_progress" | "done" | "archived";
  priority?: string;
  message_id?: string;
  channel_id?: string;
}

export async function fetchWorkspaceItems(): Promise<WorkspaceItem[]> {
  const res = await fetch("/api/workspace/items", { headers: authHeaders() });
  if (!res.ok) throw new Error(res.statusText);
  return (await res.json()).items || [];
}

export async function createWorkspaceItem(input: Partial<WorkspaceItem>): Promise<WorkspaceItem> {
  const res = await fetch("/api/workspace/items", {
    method: "POST",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export async function updateWorkspaceStatus(id: string, status: WorkspaceItem["status"]): Promise<WorkspaceItem> {
  const res = await fetch(`/api/workspace/items/${encodeURIComponent(id)}/status`, {
    method: "PUT",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  });
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export async function deleteWorkspaceItem(id: string): Promise<void> {
  const res = await fetch(`/api/workspace/items/${encodeURIComponent(id)}`, { method: "DELETE", headers: authHeaders() });
  if (!res.ok) throw new Error(res.statusText);
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

export async function uploadFileViaProxy(file: File): Promise<string> {
  const ext = getFileExtension(file.name);
  const formData = new FormData();
  formData.append("file", file);
  
  const res = await fetch(`/api/uploader/upload/proxy?ext=${ext}`, {
    method: "POST",
    headers: authHeaders(),
    body: formData,
  });
  if (res.status !== 200) throw new Error(res.statusText);
  const data = await res.json();
  return data.object_key;
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
    // Use proxy upload to bypass presigned URL DNS issues
    const objectKey = await uploadFileViaProxy(file);
    onProgress(100);
    return objectKey;
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
