"use client";

import { useState, useEffect, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Check, CheckCheck, FileDown, Pencil, Trash2, MoreHorizontal, AlertCircle, Pin, MessageSquare } from "lucide-react";
import type { Message, FilePayload, ReactionSummary } from "@/lib/constants";
import { EVENT_TEXT, EVENT_FILE, EVENT_ACTION, QUICK_EMOJIS } from "@/lib/constants";
import { fetchDownloadBlobUrl, getFileExtension, isImageExtension } from "@/lib/api";

interface PeerInfo {
  name: string;
  picture: string;
}

interface ChatMessageProps {
  message: Message & { side: "left" | "right" };
  userId: string;
  peerMap: Map<string, PeerInfo>;
  accessToken: string;
  onImageClick: (src: string) => void;
  onEdit?: (messageId: string, newPayload: string) => void;
  onDelete?: (messageId: string) => void;
  onRetry?: (clientId: string, payload: string, event: number) => void;
  onReaction?: (messageId: string, emoji: string, action: "add" | "remove") => void;
  onPin?: (messageId: string, action: "pin" | "unpin") => void;
  onReply?: (messageId: string, payload: string) => void;
}

export default function ChatMessage({
  message,
  userId,
  peerMap,
  accessToken,
  onImageClick,
  onEdit,
  onDelete,
  onRetry,
  onReaction,
  onPin,
  onReply,
}: ChatMessageProps) {
  const [fileUrl, setFileUrl] = useState<string>("");
  const [seen, setSeen] = useState(message.seen || false);
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);
  const [showMenu, setShowMenu] = useState(false);
  const [showEmojiPicker, setShowEmojiPicker] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editText, setEditText] = useState("");
  const menuRef = useRef<HTMLDivElement>(null);
  const editInputRef = useRef<HTMLTextAreaElement>(null);
  const emojiRef = useRef<HTMLDivElement>(null);

  const peer = peerMap.get(message.user_id);
  const isSelf = message.side === "right";
  const isText = message.event === EVENT_TEXT;
  const isFile = message.event === EVENT_FILE;
  const isAction = message.event === EVENT_ACTION;
  const isDeleted = message.deleted_for_all === true;
  const isEdited = (message.edited_at ?? 0) > 0;
  const isFailed = message.status === "failed";
  const isSending = message.status === "sending";
  const isDelivered = message.status === "delivered";

  useEffect(() => {
    let objectUrl = "";
    let cancelled = false;

    if (isFile && !isDeleted) {
      try {
        const payload: FilePayload = JSON.parse(message.payload);
        setFileUrl("");
        setImageLoaded(false);
        setImageError(false);

        fetchDownloadBlobUrl(payload.object_key)
          .then((url) => {
            if (cancelled) {
              URL.revokeObjectURL(url);
              return;
            }
            objectUrl = url;
            setFileUrl(url);
            setImageLoaded(false);
            setImageError(false);
          })
          .catch(() => {
            if (!cancelled) setImageError(true);
          });
      } catch {
        setImageError(true);
      }
    }

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [isFile, isDeleted, message.payload, accessToken]);

  useEffect(() => {
    setSeen(message.seen || false);
  }, [message.seen]);

  useEffect(() => {
    if (isEditing && editInputRef.current) {
      editInputRef.current.focus();
      editInputRef.current.style.height = "auto";
      editInputRef.current.style.height = editInputRef.current.scrollHeight + "px";
    }
  }, [isEditing]);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setShowMenu(false);
      }
      if (emojiRef.current && !emojiRef.current.contains(e.target as Node)) {
        setShowEmojiPicker(false);
      }
    };
    if (showMenu || showEmojiPicker) {
      document.addEventListener("mousedown", handleClickOutside);
    }
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [showMenu, showEmojiPicker]);

  const handleReact = (emoji: string) => {
    if (!message.message_id || !onReaction) return;
    const existing = message.reactions?.find((r) => r.emoji === emoji);
    const hasReacted = existing?.user_ids.includes(userId);
    onReaction(message.message_id, emoji, hasReacted ? "remove" : "add");
    setShowEmojiPicker(false);
  };

  const handleEdit = () => {
    setEditText(message.payload);
    setIsEditing(true);
    setShowMenu(false);
  };

  const handleEditSubmit = () => {
    const trimmed = editText.trim();
    if (trimmed && trimmed !== message.payload && message.message_id && onEdit) {
      onEdit(message.message_id, trimmed);
    }
    setIsEditing(false);
  };

  const handleEditKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleEditSubmit();
    }
    if (e.key === "Escape") {
      setIsEditing(false);
    }
  };

  const handleDelete = () => {
    if (message.message_id && onDelete) {
      onDelete(message.message_id);
    }
    setShowMenu(false);
  };

  if (isAction) {
    return (
      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex justify-center my-3"
      >
        <span className="px-4 py-1.5 rounded-full text-xs text-text-muted glass border border-white/5">
          {message.payload}
        </span>
      </motion.div>
    );
  }

  if (isDeleted) {
    return (
      <motion.div
        initial={{ opacity: 0, x: isSelf ? 30 : -30, y: 10 }}
        animate={{ opacity: 1, x: 0, y: 0 }}
        transition={{ type: "spring", stiffness: 300, damping: 30 }}
        className={`flex items-end mb-3 ${isSelf ? "flex-row-reverse" : ""}`}
      >
        <motion.div
          whileHover={{ scale: 1.1 }}
          className="w-8 h-8 rounded-full bg-gradient-to-br from-accent-cyan/30 to-accent-violet/30 border border-white/10 bg-cover bg-center bg-no-repeat flex-shrink-0 overflow-hidden"
          style={{
            backgroundImage: `url(${peer?.picture || ""})`,
            marginLeft: isSelf ? 12 : undefined,
            marginRight: isSelf ? undefined : 12,
          }}
        />
        <div className={`max-w-[70%] flex flex-col ${isSelf ? "items-end" : "items-start"}`}>
          <div className="relative px-4 py-2.5 rounded-2xl bg-white/5 text-text-muted italic rounded-bl-sm rounded-br-sm border border-white/5">
            <div className="flex items-center gap-2 text-sm">
              <AlertCircle className="w-3.5 h-3.5 flex-shrink-0" />
              <span>This message was deleted</span>
            </div>
          </div>
        </div>
      </motion.div>
    );
  }

  const time = message.time ? new Date(message.time) : new Date();
  const timeStr = `${String(time.getHours()).padStart(2, "0")}:${String(time.getMinutes()).padStart(2, "0")}`;

  return (
    <motion.div
      initial={{ opacity: 0, x: isSelf ? 30 : -30, y: 10 }}
      animate={{ opacity: 1, x: 0, y: 0 }}
      transition={{ type: "spring", stiffness: 300, damping: 30 }}
      className={`flex items-end mb-3 group ${isSelf ? "flex-row-reverse" : ""} ${isFailed ? "opacity-60" : ""}`}
    >
      <motion.div
        whileHover={{ scale: 1.1 }}
        className="w-8 h-8 rounded-full bg-gradient-to-br from-accent-cyan/30 to-accent-violet/30 border border-white/10 bg-cover bg-center bg-no-repeat flex-shrink-0 overflow-hidden"
        style={{
          backgroundImage: `url(${peer?.picture || ""})`,
          marginLeft: isSelf ? 12 : undefined,
          marginRight: isSelf ? undefined : 12,
        }}
      />

      <div className={`max-w-[70%] flex flex-col ${isSelf ? "items-end" : "items-start"}`}>
        {isSelf && (
          <div className="relative mb-1" ref={menuRef}>
            <motion.button
              whileHover={{ scale: 1.1 }}
              whileTap={{ scale: 0.9 }}
              onClick={() => setShowMenu(!showMenu)}
              className="opacity-0 group-hover:opacity-100 w-6 h-6 rounded-lg bg-white/5 hover:bg-white/10 flex items-center justify-center transition-opacity cursor-pointer border-none"
            >
              <MoreHorizontal className="w-3.5 h-3.5 text-text-muted" />
            </motion.button>
            <AnimatePresence>
              {showMenu && (
                <motion.div
                  initial={{ opacity: 0, scale: 0.9, y: -5 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.9, y: -5 }}
                  className="absolute right-0 top-8 z-50 glass-strong rounded-xl border border-white/10 overflow-hidden shadow-xl shadow-black/30 min-w-[140px]"
                >
                  {isText && (
                    <button
                      onClick={handleEdit}
                      className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-sm text-text-primary hover:bg-white/10 transition-colors cursor-pointer border-none bg-transparent text-left"
                    >
                      <Pencil className="w-3.5 h-3.5 text-accent-cyan" />
                      Edit
                    </button>
                  )}
                  <button
                    onClick={handleDelete}
                    className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-sm text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer border-none bg-transparent text-left"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                    Delete
                  </button>
                  {onPin && (
                    <button
                      onClick={() => {
                        if (message.message_id) {
                          onPin(message.message_id, message.is_pinned ? "unpin" : "pin");
                        }
                        setShowMenu(false);
                      }}
                      className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-sm text-text-primary hover:bg-white/10 transition-colors cursor-pointer border-none bg-transparent text-left"
                    >
                      <Pin className={`w-3.5 h-3.5 ${message.is_pinned ? "text-amber-400" : "text-text-muted"}`} />
                      {message.is_pinned ? "Unpin" : "Pin"}
                    </button>
                  )}
                  {onReply && isText && (
                    <button
                      onClick={() => {
                        if (message.message_id) {
                          onReply(message.message_id, message.payload.slice(0, 80));
                        }
                        setShowMenu(false);
                      }}
                      className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-sm text-text-primary hover:bg-white/10 transition-colors cursor-pointer border-none bg-transparent text-left"
                    >
                      <MessageSquare className="w-3.5 h-3.5 text-accent-cyan" />
                      Reply
                    </button>
                  )}
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        )}
        {!isSelf && onReply && isText && (
          <div className="relative mb-1" ref={menuRef}>
            <motion.button
              whileHover={{ scale: 1.1 }}
              whileTap={{ scale: 0.9 }}
              onClick={() => setShowMenu(!showMenu)}
              className="opacity-0 group-hover:opacity-100 w-6 h-6 rounded-lg bg-white/5 hover:bg-white/10 flex items-center justify-center transition-opacity cursor-pointer border-none"
            >
              <MoreHorizontal className="w-3.5 h-3.5 text-text-muted" />
            </motion.button>
            <AnimatePresence>
              {showMenu && (
                <motion.div
                  initial={{ opacity: 0, scale: 0.9, y: -5 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.9, y: -5 }}
                  className="absolute left-0 top-8 z-50 glass-strong rounded-xl border border-white/10 overflow-hidden shadow-xl shadow-black/30 min-w-[140px]"
                >
                  <button
                    onClick={() => {
                      if (message.message_id) {
                        onReply(message.message_id, message.payload.slice(0, 80));
                      }
                      setShowMenu(false);
                    }}
                    className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-sm text-text-primary hover:bg-white/10 transition-colors cursor-pointer border-none bg-transparent text-left"
                  >
                    <MessageSquare className="w-3.5 h-3.5 text-accent-cyan" />
                    Reply
                  </button>
                  {onPin && (
                    <button
                      onClick={() => {
                        if (message.message_id) {
                          onPin(message.message_id, message.is_pinned ? "unpin" : "pin");
                        }
                        setShowMenu(false);
                      }}
                      className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-sm text-text-primary hover:bg-white/10 transition-colors cursor-pointer border-none bg-transparent text-left"
                    >
                      <Pin className={`w-3.5 h-3.5 ${message.is_pinned ? "text-amber-400" : "text-text-muted"}`} />
                      {message.is_pinned ? "Unpin" : "Pin"}
                    </button>
                  )}
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        )}

        {isEditing ? (
          <div className="w-full">
            <textarea
              ref={editInputRef}
              value={editText}
              onChange={(e) => setEditText(e.target.value)}
              onKeyDown={handleEditKeyDown}
              onBlur={handleEditSubmit}
              className="w-full bg-white/5 rounded-2xl resize-none px-4 py-2.5 border border-accent-violet/40 text-text-primary text-sm outline-none transition-all duration-300 font-[Inter,sans-serif]"
              style={{ maxHeight: "10rem" }}
              onInput={(e) => {
                const el = e.currentTarget;
                el.style.height = "auto";
                el.style.height = el.scrollHeight + "px";
              }}
            />
            <div className="flex items-center gap-2 mt-1 px-1">
              <span className="text-[10px] text-text-muted">Escape to cancel</span>
            </div>
          </div>
        ) : (
          <>
            {message.reply_preview && (
              <div className={`mb-1 px-3 py-1.5 rounded-xl border-l-2 border-accent-violet/50 bg-white/5 max-w-full ${isSelf ? "ml-auto" : ""}`}>
                <span className="text-[10px] text-accent-violet font-medium">
                  {peerMap.get(message.reply_preview.user_id)?.name || "Unknown"}
                </span>
                <p className="text-[11px] text-text-muted truncate">{message.reply_preview.payload}</p>
              </div>
            )}
            {isText && (
              <div
                className={`relative px-4 py-2.5 rounded-2xl ${
                  isSelf
                    ? "bg-gradient-to-br from-accent-blue/80 to-accent-violet/80 text-white rounded-br-sm"
                    : "glass text-text-primary rounded-bl-sm border border-white/10"
                }`}
              >
                <div
                  className="break-words text-sm leading-relaxed"
                  dangerouslySetInnerHTML={{
                    __html: urlify(message.payload).replace(
                      /(?:\r|\n|\r\n)/g,
                      "<br>"
                    ),
                  }}
                />
              </div>
            )}

            {isFile && fileUrl && (
              <div
                className={`relative px-3 py-3 rounded-2xl ${
                  isSelf
                    ? "bg-gradient-to-br from-accent-blue/80 to-accent-violet/80 text-white rounded-br-sm"
                    : "glass text-text-primary rounded-bl-sm border border-white/10"
                }`}
              >
                {isImageExtension(getFileExtension(message.payload ? JSON.parse(message.payload).object_key : "")) ? (
                  <div className="relative overflow-hidden rounded-xl">
                    {!imageLoaded && !imageError && (
                      <div className="w-48 h-32 bg-white/5 animate-pulse rounded-xl flex items-center justify-center">
                        <span className="text-text-muted text-xs">Loading...</span>
                      </div>
                    )}
                    {imageError && (
                      <div className="w-48 h-32 bg-red-500/10 border border-red-500/20 rounded-xl flex flex-col items-center justify-center gap-1">
                        <AlertCircle className="w-5 h-5 text-red-400" />
                        <span className="text-red-400 text-xs">Failed to load</span>
                        <a href={fileUrl} target="_blank" rel="noopener noreferrer" className="text-accent-cyan text-[10px] hover:underline">
                          Open in new tab
                        </a>
                      </div>
                    )}
                    <img
                      src={fileUrl}
                      alt=""
                      className={`max-w-[280px] rounded-xl cursor-pointer hover:scale-105 transition-all duration-300 ${
                        imageLoaded && !imageError ? "opacity-100" : "absolute inset-0 opacity-0 pointer-events-none"
                      }`}
                      onClick={() => onImageClick(fileUrl)}
                      onLoad={() => setImageLoaded(true)}
                      onError={() => setImageError(true)}
                    />
                  </div>
                ) : (
                  <a
                    href={fileUrl}
                    download
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`flex items-center gap-3 px-3 py-2 rounded-xl ${
                      isSelf ? "bg-white/10 hover:bg-white/20" : "bg-white/5 hover:bg-white/10"
                    } transition-colors`}
                  >
                    <FileDown className="w-5 h-5 flex-shrink-0" />
                    <span className="break-words text-sm font-medium">
                      {JSON.parse(message.payload).file_name}
                    </span>
                  </a>
                )}
              </div>
            )}
          </>
        )}

        <div className={`flex items-center gap-1.5 mt-1 px-1 ${isSelf ? "flex-row-reverse" : ""}`}>
          {isSelf && (
            <motion.div
              initial={{ scale: 0 }}
              animate={{ scale: 1 }}
              className="flex items-center"
            >
              {isFailed ? (
                <button
                  onClick={() => message.client_id && onRetry?.(message.client_id, message.payload, message.event)}
                  className="flex items-center gap-1 cursor-pointer bg-transparent border-none p-0"
                >
                  <AlertCircle className="w-3.5 h-3.5 text-red-400" />
                </button>
              ) : isSending ? (
                <span className="text-[10px] text-text-muted">...</span>
              ) : seen ? (
                <CheckCheck className="w-3.5 h-3.5 text-accent-cyan" />
              ) : isDelivered ? (
                <CheckCheck className="w-3.5 h-3.5 text-text-secondary" />
              ) : (
                <Check className="w-3.5 h-3.5 text-text-muted" />
              )}
            </motion.div>
          )}
          {isEdited && !isEditing && (
            <span className="text-[10px] text-text-muted italic">edited</span>
          )}
          <span className="text-[10px] text-text-muted font-medium">{timeStr}</span>
        </div>

        {message.reactions && message.reactions.length > 0 && (
          <div className={`flex flex-wrap gap-1 mt-1 ${isSelf ? "flex-row-reverse" : ""}`}>
            {message.reactions.map((r) => {
              const hasReacted = r.user_ids.includes(userId);
              return (
                <button
                  key={r.emoji}
                  onClick={() => handleReact(r.emoji)}
                  className={`flex items-center gap-1 px-1.5 py-0.5 rounded-full text-xs border transition-all cursor-pointer ${
                    hasReacted
                      ? "bg-accent-violet/20 border-accent-violet/40 text-accent-violet"
                      : "bg-white/5 border-white/10 text-text-secondary hover:bg-white/10"
                  }`}
                >
                  <span>{r.emoji}</span>
                  <span className="text-[10px]">{r.count}</span>
                </button>
              );
            })}
          </div>
        )}

        <div className={`relative mt-0.5 ${isSelf ? "flex justify-end" : ""}`} ref={emojiRef}>
          <button
            onClick={() => setShowEmojiPicker(!showEmojiPicker)}
            className="opacity-0 group-hover:opacity-100 text-[10px] text-text-muted hover:text-accent-violet transition-all cursor-pointer bg-transparent border-none px-1"
          >
            +emoji
          </button>
          <AnimatePresence>
            {showEmojiPicker && (
              <motion.div
                initial={{ opacity: 0, scale: 0.9, y: 5 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.9, y: 5 }}
                className={`absolute z-50 ${isSelf ? "right-0" : "left-0"} bottom-6 glass-strong rounded-xl border border-white/10 p-2 shadow-xl shadow-black/30 flex gap-1`}
              >
                {QUICK_EMOJIS.map((emoji) => (
                  <button
                    key={emoji}
                    onClick={() => handleReact(emoji)}
                    className="w-8 h-8 flex items-center justify-center rounded-lg hover:bg-white/10 transition-colors cursor-pointer bg-transparent border-none text-lg"
                  >
                    {emoji}
                  </button>
                ))}
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </div>
    </motion.div>
  );
}

function urlify(text: string): string {
  const urlRegex = /(https?:\/\/[^\s]+)/g;
  return text.replace(urlRegex, (url) => `<a href="${url}" class="underline hover:opacity-80 transition-opacity">${url}</a>`);
}
