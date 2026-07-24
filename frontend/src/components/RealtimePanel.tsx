"use client";

import { useEffect, useMemo, useState } from "react";
import { Bookmark, Flag, ListTodo, ShieldCheck, UserRoundX } from "lucide-react";
import {
  blockSafetyUser,
  createSafetyReport,
  createWorkspaceItem,
  deleteWorkspaceItem,
  fetchWorkspaceItems,
  updateWorkspaceStatus,
  type WorkspaceItem,
} from "@/lib/api";

type Props = { userId: string; accessToken: string };

export default function RealtimePanel({ userId, accessToken }: Props) {
  const [open, setOpen] = useState<"safety" | "workspace" | null>(null);
  const [workspace, setWorkspace] = useState<WorkspaceItem[]>([]);
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");
  const wsURL = useMemo(() => {
    if (typeof window === "undefined") return "";
    const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${scheme}//${window.location.host}/api/workspace/ws?uid=${encodeURIComponent(userId)}&access_token=${encodeURIComponent(accessToken)}`;
  }, [accessToken, userId]);

  useEffect(() => {
    if (!userId || !accessToken) return;
    void fetchWorkspaceItems().then(setWorkspace).catch(() => {});
    const socket = new WebSocket(wsURL);
    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        if (message.type === "workspace.item.deleted") {
          setWorkspace((items) => items.filter((item) => item.id !== message.data.id));
        } else if (message.data?.id) {
          setWorkspace((items) => [message.data, ...items.filter((item) => item.id !== message.data.id)]);
        }
      } catch {}
    };
    return () => socket.close();
  }, [accessToken, userId, wsURL]);

  const addItem = async (kind: WorkspaceItem["kind"]) => {
    try {
      setError("");
      if (!title.trim()) throw new Error("Nhập tiêu đề trước");
      const item = await createWorkspaceItem({ kind, title: title.trim(), status: "todo" });
      setWorkspace((items) => [item, ...items]);
      setTitle("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Không thể tạo workspace item");
    }
  };

  const report = async () => {
    const target = window.prompt("ID người dùng cần báo cáo");
    if (!target) return;
    try {
      await createSafetyReport({ target_user_id: target, reason: "harassment", details: "Báo cáo từ chat" });
      setError("Đã gửi báo cáo cho Safety");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Không thể gửi báo cáo");
    }
  };

  const block = async () => {
    const target = window.prompt("ID người dùng cần chặn");
    if (!target) return;
    try {
      await blockSafetyUser(target);
      setError(`Đã chặn user ${target}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Không thể chặn user");
    }
  };

  return (
    <div className="relative flex items-center gap-1">
      <button title="Trust & Safety" onClick={() => setOpen(open === "safety" ? null : "safety")} className="rounded-lg p-2 text-emerald-300 hover:bg-white/10"><ShieldCheck className="h-4 w-4" /></button>
      <button title="Workspace" onClick={() => setOpen(open === "workspace" ? null : "workspace")} className="rounded-lg p-2 text-cyan-300 hover:bg-white/10"><ListTodo className="h-4 w-4" /></button>
      {open === "safety" && (
        <div className="absolute right-0 top-10 z-50 w-64 rounded-xl border border-white/10 bg-slate-950/95 p-3 shadow-xl">
          <p className="text-sm font-semibold text-white">Trust &amp; Safety</p>
          <p className="mt-1 text-xs text-text-muted">Báo cáo hoặc chặn người dùng mà không cần Web Push.</p>
          <div className="mt-3 flex gap-2">
            <button onClick={() => void report()} className="flex flex-1 items-center justify-center gap-1 rounded-lg bg-amber-500/15 py-2 text-xs text-amber-300"><Flag className="h-3.5 w-3.5" />Báo cáo</button>
            <button onClick={() => void block()} className="flex flex-1 items-center justify-center gap-1 rounded-lg bg-red-500/15 py-2 text-xs text-red-300"><UserRoundX className="h-3.5 w-3.5" />Chặn</button>
          </div>
          {error && <p className="mt-2 text-[11px] text-emerald-300">{error}</p>}
        </div>
      )}
      {open === "workspace" && (
        <div className="absolute right-0 top-10 z-50 w-80 rounded-xl border border-white/10 bg-slate-950/95 p-3 shadow-xl">
          <p className="text-sm font-semibold text-white">Workspace</p>
          <div className="mt-2 flex gap-1">
            <input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="Task hoặc note mới" className="min-w-0 flex-1 rounded-lg bg-white/5 px-2 py-1.5 text-xs text-white outline-none" />
            <button onClick={() => void addItem("task")} title="Tạo task" className="rounded-lg bg-cyan-500/20 p-2 text-cyan-300"><ListTodo className="h-3.5 w-3.5" /></button>
            <button onClick={() => void addItem("bookmark")} title="Tạo bookmark" className="rounded-lg bg-violet-500/20 p-2 text-violet-300"><Bookmark className="h-3.5 w-3.5" /></button>
          </div>
          <div className="mt-2 max-h-56 space-y-1 overflow-y-auto">
            {workspace.length === 0 && <p className="py-4 text-center text-xs text-text-muted">Chưa có task/note</p>}
            {workspace.map((item) => <div key={item.id} className="flex items-center gap-2 rounded-lg bg-white/5 p-2"><span className="min-w-0 flex-1 truncate text-xs text-white">{item.title}</span><button onClick={() => void updateWorkspaceStatus(item.id, item.status === "done" ? "todo" : "done").then((updated) => setWorkspace((items) => items.map((value) => value.id === updated.id ? updated : value)))} className="text-[10px] text-emerald-300">{item.status}</button><button onClick={() => void deleteWorkspaceItem(item.id).then(() => setWorkspace((items) => items.filter((value) => value.id !== item.id)))} className="text-[10px] text-red-300">×</button></div>)}
          </div>
          {error && <p className="mt-2 text-[11px] text-red-400">{error}</p>}
        </div>
      )}
    </div>
  );
}
