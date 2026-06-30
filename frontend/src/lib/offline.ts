const DB_NAME = "nexuschat_offline";
const DB_VERSION = 2;
const MESSAGES_STORE = "messages";
const OUTBOX_STORE = "outbox";
const SYNC_STORE = "sync";

let db: IDBDatabase | null = null;

export function openDB(): Promise<IDBDatabase> {
  if (db) return Promise.resolve(db);
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      const database = request.result;
      if (!database.objectStoreNames.contains(MESSAGES_STORE)) {
        const messages = database.createObjectStore(MESSAGES_STORE, { keyPath: "id" });
        messages.createIndex("channel_id", "channel_id");
      } else {
        const tx = request.transaction;
        const messages = tx?.objectStore(MESSAGES_STORE);
        if (messages && !messages.indexNames.contains("channel_id")) {
          messages.createIndex("channel_id", "channel_id");
        }
      }
      if (!database.objectStoreNames.contains(OUTBOX_STORE)) {
        const outbox = database.createObjectStore(OUTBOX_STORE, { keyPath: "id" });
        outbox.createIndex("timestamp", "timestamp");
      }
      if (!database.objectStoreNames.contains(SYNC_STORE)) {
        database.createObjectStore(SYNC_STORE, { keyPath: "key" });
      }
    };
    request.onsuccess = () => {
      db = request.result;
      resolve(db);
    };
    request.onerror = () => reject(request.error);
  });
}

export interface QueuedAction {
  id: string;
  type: string;
  payload: any;
  timestamp: number;
  retries: number;
}

export interface CachedMessage {
  id: string;
  channel_id: string;
  data: any;
  timestamp: number;
}

export async function cacheMessages(channelId: string, messages: any[]): Promise<void> {
  const database = await openDB();
  const tx = database.transaction(MESSAGES_STORE, "readwrite");
  const store = tx.objectStore(MESSAGES_STORE);
  for (const msg of messages) {
    if (!msg.message_id) continue;
    store.put({
      id: `${channelId}:${msg.message_id}`,
      channel_id: channelId,
      data: msg,
      timestamp: Date.now(),
    });
  }
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

export async function getCachedMessages(channelId: string): Promise<any[]> {
  const database = await openDB();
  const tx = database.transaction(MESSAGES_STORE, "readonly");
  const store = tx.objectStore(MESSAGES_STORE);
  const index = store.index("channel_id");
  const range = IDBKeyRange.only(channelId);
  return new Promise((resolve, reject) => {
    const request = index.getAll(range);
    request.onsuccess = () => {
      const results = (request.result as CachedMessage[])
        .sort((a, b) => a.timestamp - b.timestamp)
        .map((r) => r.data);
      resolve(results);
    };
    request.onerror = () => reject(request.error);
  });
}

export async function enqueueAction(type: string, payload: any): Promise<void> {
  const database = await openDB();
  const tx = database.transaction(OUTBOX_STORE, "readwrite");
  const store = tx.objectStore(OUTBOX_STORE);
  store.put({
    id: `action-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    type,
    payload,
    timestamp: Date.now(),
    retries: 0,
  });
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

export async function getPendingActions(): Promise<QueuedAction[]> {
  const database = await openDB();
  const tx = database.transaction(OUTBOX_STORE, "readonly");
  const store = tx.objectStore(OUTBOX_STORE);
  return new Promise((resolve, reject) => {
    const request = store.getAll();
    request.onsuccess = () => {
      const results = (request.result as QueuedAction[]).sort((a, b) => a.timestamp - b.timestamp);
      resolve(results);
    };
    request.onerror = () => reject(request.error);
  });
}

export async function removeAction(id: string): Promise<void> {
  const database = await openDB();
  const tx = database.transaction(OUTBOX_STORE, "readwrite");
  const store = tx.objectStore(OUTBOX_STORE);
  store.delete(id);
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

export async function setSyncCursor(cursor: string): Promise<void> {
  const database = await openDB();
  const tx = database.transaction(SYNC_STORE, "readwrite");
  const store = tx.objectStore(SYNC_STORE);
  store.put({ key: "cursor", value: cursor });
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

export async function getSyncCursor(): Promise<string> {
  const database = await openDB();
  const tx = database.transaction(SYNC_STORE, "readonly");
  const store = tx.objectStore(SYNC_STORE);
  return new Promise((resolve, reject) => {
    const request = store.get("cursor");
    request.onsuccess = () => {
      resolve(request.result?.value || "");
    };
    request.onerror = () => reject(request.error);
  });
}

export async function clearAll(): Promise<void> {
  const database = await openDB();
  const stores = [MESSAGES_STORE, OUTBOX_STORE, SYNC_STORE];
  const tx = database.transaction(stores, "readwrite");
  for (const storeName of stores) {
    tx.objectStore(storeName).clear();
  }
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}
