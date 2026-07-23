import { getPendingActions, removeAction, cacheMessages, getCachedMessages, enqueueAction } from "./offline";
import type { Message } from "./constants";
import { EVENT_TEXT, EVENT_FILE, EVENT_EDIT, EVENT_DELETE, EVENT_REACTION } from "./constants";

type SendFn = (msg: object) => void;

export class SyncEngine {
  private sendFn: SendFn;
  private isOnline: boolean;
  private isSyncing: boolean;

  constructor(sendFn: SendFn) {
    this.sendFn = sendFn;
    this.isOnline = typeof navigator !== "undefined" ? navigator.onLine : true;
    this.isSyncing = false;

    if (typeof window !== "undefined") {
      window.addEventListener("online", () => {
        this.isOnline = true;
        this.sync();
      });
      window.addEventListener("offline", () => {
        this.isOnline = false;
      });
    }
  }

  async queueAction(eventType: number, payload: string, userId: string): Promise<void> {
    await enqueueAction("ws_send", { event: eventType, user_id: userId, payload });
    if (this.isOnline) {
      await this.sync();
    }
  }

  async cacheIncomingMessages(channelId: string, messages: Message[]): Promise<void> {
    await cacheMessages(channelId, messages);
  }

  async getCachedMessages(channelId: string): Promise<Message[]> {
    return getCachedMessages(channelId);
  }

  async sync(): Promise<void> {
    if (!this.isOnline || this.isSyncing) return;
    this.isSyncing = true;

    try {
      const actions = await getPendingActions();
      for (const action of actions) {
        if (action.type === "ws_send") {
          try {
            this.sendFn(action.payload);
            await removeAction(action.id);
          } catch {
            break;
          }
        }
      }
    } finally {
      this.isSyncing = false;
    }
  }

  get onlineStatus(): boolean {
    return this.isOnline;
  }
}
