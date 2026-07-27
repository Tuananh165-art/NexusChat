"use client";

import { useEffect, useState } from "react";
import { Copy, DoorOpen, Hash, Plus, UsersRound, LogOut } from "lucide-react";
import { ACCESS_TOKEN_KEY, type ConversationContext } from "@/lib/constants";
import { createRoom, fetchRooms, joinRoom, leaveRoom, openRoom, type ChatRoom } from "@/lib/api";

type Props = {
  userId: string;
  onOpenRoom: (accessToken: string, channelId: string, context?: ConversationContext) => void;
};

export default function RoomsPanel({ userId, onOpenRoom }: Props) {
  const [open, setOpen] = useState(false);
  const [rooms, setRooms] = useState<ChatRoom[]>([]);
  const [name, setName] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const loadRooms = async () => {
    if (!userId) return;
    try {
      setRooms(await fetchRooms());
    } catch {
      setMessage("Rooms could not be loaded");
    }
  };

  useEffect(() => {
    if (open) void loadRooms();
  }, [open, userId]);

  const handleCreate = async () => {
    if (!name.trim() || busy) return;
    setBusy(true);
    setMessage("");
    try {
      const room = await createRoom({ name: name.trim() });
      setRooms((items) => [room, ...items.filter((item) => item.channel_id !== room.channel_id)]);
      setName("");
      setMessage(`Room created. Invite code: ${room.invite_code}`);
    } catch {
      setMessage("The room could not be created");
    } finally {
      setBusy(false);
    }
  };

  const handleJoin = async () => {
    if (!inviteCode.trim() || busy) return;
    setBusy(true);
    setMessage("");
    try {
      const room = await joinRoom(inviteCode.trim());
      setRooms((items) => [room, ...items.filter((item) => item.channel_id !== room.channel_id)]);
      setInviteCode("");
      setMessage(`Joined ${room.name}`);
    } catch {
      setMessage("That invite code is invalid or the room does not exist");
    } finally {
      setBusy(false);
    }
  };

  const handleOpen = async (room: ChatRoom) => {
    if (busy) return;
    setBusy(true);
    setMessage("");
    try {
      const result = await openRoom(room.channel_id);
      localStorage.setItem(ACCESS_TOKEN_KEY, result.access_token);
      onOpenRoom(result.access_token, result.channel_id, {
        channel_id: result.channel_id,
        kind: "group",
        title: room.name,
        avatar: room.avatar,
        member_count: room.member_count,
      });
    } catch {
      setMessage("The room could not be opened");
    } finally {
      setBusy(false);
    }
  };

  const handleLeave = async (room: ChatRoom) => {
    if (busy || room.role === "owner") {
      if (room.role === "owner") setMessage("Owners must transfer ownership before leaving");
      return;
    }
    setBusy(true);
    try {
      await leaveRoom(room.channel_id);
      setRooms((items) => items.filter((item) => item.channel_id !== room.channel_id));
      setMessage(`Left ${room.name}`);
    } catch {
      setMessage("The room could not be left");
    } finally {
      setBusy(false);
    }
  };

  const copyInvite = async (room: ChatRoom) => {
    try {
      await navigator.clipboard.writeText(room.invite_code);
      setMessage(`Copied code ${room.invite_code}`);
    } catch {
      setMessage(`Invite code: ${room.invite_code}`);
    }
  };

  return (
    <div className="relative flex items-center">
      <button
        title="Rooms"
        onClick={() => setOpen((value) => !value)}
        className="rounded-lg p-2 text-cyan-300 hover:bg-white/10"
      >
        <UsersRound className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute right-0 top-10 z-50 w-80 rounded-xl border border-white/10 bg-slate-950/95 p-3 shadow-xl">
          <div className="flex items-center justify-between">
            <p className="text-sm font-semibold text-white">Rooms</p>
            <span className="text-[11px] text-text-muted">{rooms.length} room</span>
          </div>
          <div className="mt-3 flex gap-1">
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="New room name"
              className="min-w-0 flex-1 rounded-lg bg-white/5 px-2 py-1.5 text-xs text-white outline-none"
            />
            <button onClick={() => void handleCreate()} title="Create room" className="rounded-lg bg-cyan-500/20 p-2 text-cyan-300 disabled:opacity-50" disabled={busy}>
              <Plus className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="mt-2 flex gap-1">
            <input
              value={inviteCode}
              onChange={(event) => setInviteCode(event.target.value.toUpperCase())}
              placeholder="Enter invite code"
              className="min-w-0 flex-1 rounded-lg bg-white/5 px-2 py-1.5 text-xs uppercase text-white outline-none"
            />
            <button onClick={() => void handleJoin()} title="Join room" className="rounded-lg bg-emerald-500/20 p-2 text-emerald-300 disabled:opacity-50" disabled={busy}>
              <Hash className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="mt-3 max-h-64 space-y-2 overflow-y-auto">
            {rooms.length === 0 && <p className="rounded-lg bg-white/5 px-3 py-4 text-center text-xs text-text-muted">No group rooms yet</p>}
            {rooms.map((room) => (
              <div key={room.channel_id} className="rounded-lg bg-white/5 p-2">
                <div className="flex items-center gap-2">
                  <div className="h-8 w-8 rounded-lg bg-cyan-500/15 bg-cover bg-center text-cyan-300" style={{ backgroundImage: `url(${room.avatar || `https://api.dicebear.com/7.x/identicon/svg?seed=${room.channel_id}`})` }}>
                    {!room.avatar && <UsersRound className="m-2 h-4 w-4" />}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-xs font-semibold text-white">{room.name}</p>
                    <p className="text-[11px] text-text-muted">{room.member_count || 1} members · {room.role || "member"}</p>
                  </div>
                  <div className="flex items-center gap-1">
                    <button onClick={() => void handleOpen(room)} title="Open room" className="rounded-lg bg-cyan-500/20 p-2 text-cyan-300">
                      <DoorOpen className="h-3.5 w-3.5" />
                    </button>
                    {room.role !== "owner" && <button onClick={() => void handleLeave(room)} title="Leave room" className="rounded-lg bg-red-500/15 p-2 text-red-300">
                      <LogOut className="h-3.5 w-3.5" />
                    </button>}
                  </div>
                </div>
                <button onClick={() => void copyInvite(room)} className="mt-2 flex w-full items-center justify-center gap-1 rounded-lg bg-white/5 px-2 py-1 text-[11px] text-text-secondary hover:text-white">
                  <Copy className="h-3 w-3" />
                  {room.invite_code}
                </button>
              </div>
            ))}
          </div>
          {message && <p className="mt-2 text-[11px] text-emerald-300">{message}</p>}
        </div>
      )}
    </div>
  );
}
