"use client";

import { useCallback, useEffect, useState } from "react";
import { Bell, Check, Loader2, MessageCircle, UserPlus, X } from "lucide-react";
import { acceptFriendRequest, createDirectChat, declineFriendRequest, fetchFriendRequests, fetchNotifications, fetchUser, markAllNotificationsRead, markNotificationRead, type FriendRequest, type UserNotification, type UserSearchResult } from "@/lib/api";
import { ACCESS_TOKEN_KEY, CONVERSATION_CONTEXT_KEY } from "@/lib/constants";

type RequestWithUser = FriendRequest & { sender?: UserSearchResult };

type Props = { compact?: boolean };

export default function FriendRequestsPanel({ compact = false }: Props) {
  const [open, setOpen] = useState(false);
  const [requests, setRequests] = useState<RequestWithUser[]>([]);
  const [notifications, setNotifications] = useState<UserNotification[]>([]);
  const [busyId, setBusyId] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const incoming = await fetchFriendRequests();
      const enriched = await Promise.all(incoming.map(async (request) => {
        try {
          const sender = await fetchUser(request.from_user_id);
          return { ...request, sender };
        } catch {
          return request;
        }
      }));
      setRequests(enriched);
      try { setNotifications(await fetchNotifications()); } catch { /* request list remains usable */ }
      setError("");
    } catch {
      setError("Friend requests could not be loaded.");
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15000);
    return () => window.clearInterval(timer);
  }, [load]);

  const handleToggleOpen = () => {
    const nextOpen = !open;
    setOpen(nextOpen);
    if (!nextOpen) return;
    setNotifications((items) => items.map((item) => ({ ...item, read: true })));
    void markAllNotificationsRead().catch(() => {
      // The local state is still cleared; the next poll will restore any
      // notifications that could not be persisted as read.
    });
  };

  const updateRequest = async (request: RequestWithUser, accept: boolean) => {
    if (busyId) return;
    setBusyId(request.from_user_id);
    try {
      if (accept) await acceptFriendRequest(request.from_user_id);
      else await declineFriendRequest(request.from_user_id);
      setRequests((items) => items.filter((item) => item.from_user_id !== request.from_user_id));
    } catch {
      setError("The friend request could not be updated");
    } finally {
      setBusyId("");
    }
  };

  const openDirectChatInvite = async (notification: UserNotification) => {
    if (busyId) return;
    setBusyId(notification.id);
    setError("");
    try {
      const peer = await fetchUser(notification.actor_id);
      const result = await createDirectChat(notification.actor_id);
      localStorage.setItem(ACCESS_TOKEN_KEY, result.access_token);
      localStorage.setItem(CONVERSATION_CONTEXT_KEY, JSON.stringify({
        channel_id: result.channel_id,
        kind: "direct",
        title: peer.name,
        avatar: peer.picture,
        peer_user_id: notification.actor_id,
        peer_handle: peer.handle,
      }));
      await markNotificationRead(notification.id);
      window.location.assign("/chat");
    } catch {
      setError("The conversation invite could not be opened. Please refresh and try again.");
    } finally {
      setBusyId("");
    }
  };

  const notificationText = (item: UserNotification) => {
    if (item.type === "direct_chat_invite") return "A friend invited you to chat";
    if (item.type === "friend_request_accepted") return "A friend request was accepted";
    if (item.type === "friend_request_declined") return "A friend request was declined";
    return "You have a new friend request";
  };

  return (
    <div className="relative">
      <button title="Friend requests" onClick={handleToggleOpen} className="relative rounded-lg p-2 text-pink-300 hover:bg-white/10">
        <Bell className="h-4 w-4" />
        {notifications.filter((item) => !item.read).length > 0 && <span className="absolute -right-0.5 -top-0.5 min-w-4 rounded-full bg-pink-500 px-1 text-center text-[9px] font-bold text-white">{notifications.filter((item) => !item.read).length > 9 ? "9+" : notifications.filter((item) => !item.read).length}</span>}
      </button>
      {open && <div className={`absolute right-0 top-10 z-50 rounded-xl border border-white/10 bg-slate-950/95 p-3 shadow-xl ${compact ? "w-80" : "w-96"}`}>
        <div className="flex items-center justify-between"><p className="text-sm font-semibold text-white">Friend requests</p><span className="text-[11px] text-text-muted">{requests.length} pending</span></div>
        <div className="mt-3 max-h-72 space-y-2 overflow-y-auto">
          {requests.length === 0 && <p className="rounded-lg bg-white/5 px-3 py-5 text-center text-xs text-text-muted">No new requests</p>}
          {requests.map((request) => {
            const sender = request.sender;
            const picture = sender?.picture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${request.from_user_id}`;
            return <div key={request.from_user_id} className="flex items-center gap-2 rounded-lg bg-white/5 p-2">
              <div className="h-9 w-9 shrink-0 rounded-xl bg-cover bg-center" style={{ backgroundImage: `url(${picture})` }} />
              <div className="min-w-0 flex-1"><p className="truncate text-xs font-semibold text-white">{sender?.name || `User ${request.from_user_id}`}</p><p className="truncate text-[11px] text-accent-violet">{sender?.handle ? `@${sender.handle}` : `ID: ${request.from_user_id}`}</p></div>
              <button title="Accept" disabled={Boolean(busyId)} onClick={() => void updateRequest(request, true)} className="rounded-lg bg-emerald-500/20 p-2 text-emerald-300 disabled:opacity-50"><Check className="h-3.5 w-3.5" /></button>
              <button title="Decline" disabled={Boolean(busyId)} onClick={() => void updateRequest(request, false)} className="rounded-lg bg-red-500/15 p-2 text-red-300 disabled:opacity-50"><X className="h-3.5 w-3.5" /></button>
            </div>;
          })}
        </div>
        {notifications.length > 0 && <div className="mt-3 border-t border-white/10 pt-2"><p className="text-[11px] font-semibold text-text-secondary">Recent activity</p><div className="mt-1 space-y-1">{notifications.slice(0, 5).map((item) => <div key={item.id} className="flex items-center gap-2 text-[11px] text-text-muted"><span className="min-w-0 flex-1">{notificationText(item)} · ID {item.actor_id}</span>{item.type === "direct_chat_invite" && <button onClick={() => void openDirectChatInvite(item)} disabled={Boolean(busyId)} className="shrink-0 rounded-md bg-cyan-500/15 px-2 py-1 text-cyan-300 hover:bg-cyan-500/25 disabled:opacity-50"><MessageCircle className="mr-1 inline h-3 w-3" />Open chat</button>}</div>)}</div></div>}
        {error && <p className="mt-2 text-[11px] text-red-300">{error}</p>}
        <button onClick={() => void load()} disabled={Boolean(busyId)} className="mt-2 flex w-full items-center justify-center gap-1 rounded-lg bg-white/5 py-1.5 text-[11px] text-text-secondary hover:text-white">{busyId ? <Loader2 className="h-3 w-3 animate-spin" /> : <UserPlus className="h-3 w-3" />} Refresh</button>
      </div>}
    </div>
  );
}
