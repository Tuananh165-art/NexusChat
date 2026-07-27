"use client";

import { useState } from "react";
import { MessageCircle, Search, UserPlus, Clock } from "lucide-react";
import { createDirectChat, fetchFriendshipStatus, searchUsers, sendFriendRequest, type UserSearchResult } from "@/lib/api";
import { ACCESS_TOKEN_KEY, type ConversationContext } from "@/lib/constants";
import FriendRequestsPanel from "@/components/FriendRequestsPanel";

type Props = { onOpenChat: (accessToken: string, channelId: string, context?: ConversationContext) => void };

export default function UserDiscoveryPanel({ onOpenChat }: Props) {
  const [query, setQuery] = useState("");
  const [users, setUsers] = useState<UserSearchResult[]>([]);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const handleSearch = async () => {
    if (!query.trim() || busy) return;
    setBusy(true); setMessage("");
    try {
      const result = await searchUsers(query.trim());
      const withStatus = await Promise.all(result.map(async (user) => {
        try { return { ...user, friend_status: (await fetchFriendshipStatus(user.id)).status }; }
        catch { return { ...user, friend_status: "none" as const }; }
      }));
      setUsers(withStatus);
      if (result.length === 0) setMessage("No users found.");
    } catch { setMessage("Users could not be searched."); }
    finally { setBusy(false); }
  };

  const handleChat = async (user: UserSearchResult) => {
    if (busy || user.friend_status !== "accepted") { setMessage("You can chat after the other person accepts your request."); return; }
    setBusy(true); setMessage("");
    try {
      const result = await createDirectChat(user.id);
      localStorage.setItem(ACCESS_TOKEN_KEY, result.access_token);
      onOpenChat(result.access_token, result.channel_id, { channel_id: result.channel_id, kind: "direct", title: user.name, avatar: user.picture, peer_user_id: user.id, peer_handle: user.handle });
    } catch { setMessage("The conversation could not be opened."); }
    finally { setBusy(false); }
  };

  const handleAddFriend = async (user: UserSearchResult) => {
    try {
      await sendFriendRequest(user.id);
      setUsers((items) => items.map((item) => item.id === user.id ? { ...item, friend_status: "pending_outgoing" } : item));
      setMessage(`Friend request sent to ${user.name}. You can chat after it is accepted.`);
    } catch { setMessage("The friend request could not be sent."); }
  };

  return (
    <div className="glass-strong w-full max-w-lg rounded-3xl p-6">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="flex-1 text-center"><h2 className="text-xl font-bold text-gradient">Find someone to chat with</h2><p className="mt-1 text-xs text-text-secondary">Search by name, @handle, or public ID.</p></div>
        <FriendRequestsPanel compact />
      </div>
      <div className="flex gap-2">
        <input value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void handleSearch(); }} placeholder="Name, @handle, or ID" className="min-w-0 flex-1 rounded-xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white outline-none placeholder:text-text-muted focus:border-accent-violet/50" />
        <button aria-label="Search users" onClick={() => void handleSearch()} disabled={busy} className="rounded-xl bg-accent-violet/20 px-3 text-accent-violet transition hover:bg-accent-violet/30 disabled:opacity-50"><Search className="h-4 w-4" /></button>
      </div>
      <div className="mt-3 space-y-2">
        {users.map((user) => (
          <div key={user.id} className="flex items-center gap-3 rounded-xl bg-white/5 p-3">
            <div className="h-9 w-9 rounded-xl bg-cover bg-center" style={{ backgroundImage: `url(${user.picture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${user.id}`})` }} />
            <div className="min-w-0 flex-1"><p className="truncate text-sm font-medium text-white">{user.name}</p><p className="text-[11px] text-accent-violet">@{user.handle || user.id}</p>{user.friend_status === "pending_outgoing" && <p className="text-[10px] text-amber-300">Waiting for acceptance</p>}{user.friend_status === "pending_incoming" && <p className="text-[10px] text-emerald-300">This person sent you a request</p>}{user.friend_status === "declined" && <p className="text-[10px] text-text-muted">Previous request declined; you can send it again</p>}<p className="text-[11px] text-text-muted">ID: {user.id}{user.username ? ` · username: ${user.username}` : ""}</p></div>
            {(user.friend_status === "none" || user.friend_status === "declined") && <button title="Send friend request" onClick={() => void handleAddFriend(user)} className="rounded-lg bg-pink-500/15 p-2 text-pink-300 transition hover:bg-pink-500/25"><UserPlus className="h-4 w-4" /></button>}
            {user.friend_status === "pending_outgoing" && <button title="Waiting for acceptance" disabled className="rounded-lg bg-pink-500/15 p-2 text-pink-300 opacity-50"><Clock className="h-4 w-4" /></button>}
            <button title={user.friend_status === "accepted" ? "Start chat" : "Request acceptance before chatting"} disabled={user.friend_status !== "accepted"} onClick={() => void handleChat(user)} className="rounded-lg bg-cyan-500/15 p-2 text-cyan-300 transition hover:bg-cyan-500/25 disabled:cursor-not-allowed disabled:opacity-30"><MessageCircle className="h-4 w-4" /></button>
          </div>
        ))}
      </div>
      {message && <p className="mt-3 text-center text-xs text-emerald-300">{message}</p>}
    </div>
  );
}
