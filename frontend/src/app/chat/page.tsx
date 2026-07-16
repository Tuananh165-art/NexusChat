"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import {
  Send,
  Paperclip,
  LogOut,
  Wifi,
  WifiOff,
  MessageCircle,
  Users,
  Loader2,
  Upload,
  Pin,
  MessageSquare,
  X,
  Search,
  Image,
  Bell,
  BellOff,
  AtSign,
  Camera,
  Save,
  Sparkles,
} from "lucide-react";
import {
  ACCESS_TOKEN_KEY,
  EVENT_TEXT,
  EVENT_ACTION,
  EVENT_SEEN,
  EVENT_FILE,
  EVENT_EDIT,
  EVENT_DELETE,
  EVENT_REACTION,
  EVENT_PIN,
} from "@/lib/constants";
import type { Message, FilePayload, PinnedMessage } from "@/lib/constants";
import {
  deleteChannel,
  fetchUser,
  fetchChannelUsers,
  fetchChannelMessages,
  fetchOnlineUsers,
  fetchPinnedMessages,
  fetchMyRole,
  updateUserProfile,
  chunkedUpload,
  searchMessages,
  fetchMediaMessages,
  fetchNotificationPrefs,
  setNotificationPrefs,
  rewriteWithAI,
  getFileExtension,
  isImageExtension,
  mobileCheck,
} from "@/lib/api";
import { cacheMessages, clearAll, getCachedMessages } from "@/lib/offline";
import { useWebSocket } from "@/hooks/useWebSocket";
import ChatMessage from "@/components/ChatMessage";
import ImageModal from "@/components/ImageModal";
import TypingIndicator from "@/components/TypingIndicator";
import AnimatedBackground from "@/components/AnimatedBackground";
import GalleryItem from "@/components/GalleryItem";

interface PeerInfo {
  name: string;
  picture: string;
}

type ConnectionStatus = "connecting" | "connected" | "disconnected";
type Msg = Message & { side: "left" | "right" };

export default function ChatPage() {
  const router = useRouter();
  const [messages, setMessages] = useState<Msg[]>([]);
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>("disconnected");
  const [onlineUsers, setOnlineUsers] = useState<string>("only you");
  const [peerTyping, setPeerTyping] = useState(false);
  const [modalImage, setModalImage] = useState<string | null>(null);
  const [userId, setUserId] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [pageState, setPageState] = useState<string>("");
  const [hasMoreMessages, setHasMoreMessages] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const [uploads, setUploads] = useState<{ id: string; name: string; progress: number; status: "uploading" | "done" | "error" }[]>([]);
  const [draftText, setDraftText] = useState("");
  const draftDebounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const [pinnedMessages, setPinnedMessages] = useState<PinnedMessage[]>([]);
  const [replyTo, setReplyTo] = useState<{ messageId: string; preview: string } | null>(null);
  const [myRole, setMyRole] = useState<string>("member");
  const [showSearch, setShowSearch] = useState(false);
  const [showGallery, setShowGallery] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<Msg[]>([]);
  const [galleryMessages, setGalleryMessages] = useState<Msg[]>([]);
  const [galleryType, setGalleryType] = useState("all");
  const [isSearching, setIsSearching] = useState(false);
  const [notifPref, setNotifPref] = useState<string>("all");
  const [showNotifSettings, setShowNotifSettings] = useState(false);
  const [notificationPermission, setNotificationPermission] = useState<NotificationPermission | "unsupported">("default");
  const [showProfileSettings, setShowProfileSettings] = useState(false);
  const [profileName, setProfileName] = useState("");
  const [profilePicture, setProfilePicture] = useState("");
  const [profileError, setProfileError] = useState("");
  const [isSavingProfile, setIsSavingProfile] = useState(false);
  const [showAIPanel, setShowAIPanel] = useState(false);
  const [isAIWorking, setIsAIWorking] = useState(false);
  const [aiError, setAIError] = useState("");

  const peerMapRef = useRef<Map<string, PeerInfo>>(new Map());
  const peerMessagesRef = useRef<Message[]>([]);
  const isPageHiddenRef = useRef(false);
  const typingTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const userIdRef = useRef("");
  const accessTokenRef = useRef("");
  const chatContainerRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const prevScrollHeightRef = useRef(0);
  const isAtBottomRef = useRef(true);
  const isTypingRef = useRef(false);

  const getChannelCacheKey = useCallback(() => {
    return accessTokenRef.current ? `channel:${accessTokenRef.current}` : "";
  }, []);

  const cacheChannelMessages = useCallback(async (items: Message[]) => {
    const key = getChannelCacheKey();
    if (!key || items.length === 0) return;
    try {
      await cacheMessages(key, items);
    } catch (err) {
      console.error("Cache messages failed:", err);
    }
  }, [getChannelCacheKey]);

  const mergeMessages = useCallback((current: Msg[], incoming: Msg[]) => {
    const byId = new Map<string, Msg>();
    const withoutId: Msg[] = [];

    for (const msg of [...current, ...incoming]) {
      if (msg.message_id) {
        byId.set(msg.message_id, msg);
      } else {
        withoutId.push(msg);
      }
    }

    return [...withoutId, ...Array.from(byId.values())].sort((a, b) => {
      const at = Number(a.time) || new Date(a.time || 0).getTime();
      const bt = Number(b.time) || new Date(b.time || 0).getTime();
      return at - bt;
    });
  }, []);

  useEffect(() => {
    userIdRef.current = userId;
  }, [userId]);

  useEffect(() => {
    accessTokenRef.current = accessToken;
  }, [accessToken]);

  const getDraftKey = useCallback(() => {
    const token = accessTokenRef.current;
    return token ? `rc:draft:${token.slice(0, 20)}` : "rc:draft:default";
  }, []);

  const saveDraft = useCallback((text: string) => {
    if (draftDebounceRef.current) clearTimeout(draftDebounceRef.current);
    draftDebounceRef.current = setTimeout(() => {
      try {
        if (text.trim()) {
          localStorage.setItem(getDraftKey(), text);
        } else {
          localStorage.removeItem(getDraftKey());
        }
      } catch {}
    }, 500);
  }, [getDraftKey]);

  const loadDraft = useCallback(() => {
    try {
      return localStorage.getItem(getDraftKey()) || "";
    } catch {
      return "";
    }
  }, [getDraftKey]);

  const clearDraft = useCallback(() => {
    try {
      localStorage.removeItem(getDraftKey());
    } catch {}
  }, [getDraftKey]);

  const getPeerInfo = useCallback(async (uid: string): Promise<PeerInfo> => {
    if (peerMapRef.current.has(uid)) {
      return peerMapRef.current.get(uid)!;
    }
    try {
      const info = await fetchUser(uid);
      const peer: PeerInfo = {
        name: info.name,
        picture: info.picture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${uid}`,
      };
      peerMapRef.current.set(uid, peer);
      return peer;
    } catch {
      return { name: "Unknown", picture: `https://api.dicebear.com/7.x/pixel-art/svg?seed=${uid}` };
    }
  }, []);

  const markMessagesAsSeen = useCallback(() => {
    const ws = useWebSocketRef.current;
    if (!ws) return;
    for (let i = peerMessagesRef.current.length - 1; i >= 0; i--) {
      if (peerMessagesRef.current[i].seen) break;
      peerMessagesRef.current[i].seen = true;
      ws.send({
        event: EVENT_SEEN,
        user_id: peerMessagesRef.current[i].user_id!,
        payload: peerMessagesRef.current[i].message_id!,
      });
    }
  }, []);

  const updateOnlineUsers = useCallback(async () => {
    try {
      const result = await fetchOnlineUsers();
      const names: string[] = [];
      for (const uid of result.user_ids) {
        if (uid === userIdRef.current) continue;
        const peer = await getPeerInfo(uid);
        names.push(peer.name);
      }
      setOnlineUsers(names.length > 0 ? names.join(", ") : "only you");
    } catch {
    }
  }, [getPeerInfo]);

  const processMessage = useCallback(
    async (m: Message): Promise<Msg | null> => {
      const uid = userIdRef.current;
      const side = m.user_id === uid ? "right" : "left";

      switch (m.event) {
        case EVENT_TEXT:
          return { ...m, side };

        case EVENT_ACTION:
          if (m.payload === "istyping" && m.user_id !== uid) {
            setPeerTyping(true);
            return null;
          }
          if (m.payload === "endtyping" && m.user_id !== uid) {
            setPeerTyping(false);
            return null;
          }
          if (m.payload.startsWith("delivered:") && m.user_id !== uid) {
            const deliveredMsgId = m.payload.split(":")[1];
            setMessages((prev) =>
              prev.map((msg) =>
                msg.message_id === deliveredMsgId && msg.status !== "delivered"
                  ? { ...msg, status: "delivered" as const }
                  : msg
              )
            );
            return null;
          }
          if (m.payload === "waiting" || m.payload === "joined" || m.payload === "offline") {
            await updateOnlineUsers();
            return null;
          }
          if (m.payload === "leaved") {
            await updateOnlineUsers();
            if (m.user_id !== uid) {
              const peer = await getPeerInfo(m.user_id!);
              return { ...m, payload: `${peer.name} leaved, channel closed`, side };
            }
            return null;
          }
          return null;

        case EVENT_FILE:
          return { ...m, side };

        default:
          return null;
      }
    },
    [getPeerInfo, updateOnlineUsers]
  );

  const useWebSocketRef = useRef<ReturnType<typeof useWebSocket> | null>(null);

  const shouldNotifyIncomingMessage = useCallback(
    (m: Message) => {
      if (!isPageHiddenRef.current || notifPref === "mute") return false;
      if (notifPref === "mentions") {
        if (m.event !== EVENT_TEXT) return false;
        const myName = peerMapRef.current.get(userIdRef.current)?.name || "";
        return Boolean(myName && m.payload.toLowerCase().includes(myName.toLowerCase()));
      }
      return true;
    },
    [notifPref]
  );

  const handleWebSocketMessage = useCallback(
    async (m: Message) => {
      const uid = userIdRef.current;

      if (m.event === EVENT_ACTION) {
        if (m.payload === "leaved") {
          await updateOnlineUsers();
          setConnectionStatus("disconnected");
        }
        if (["waiting", "joined", "offline"].includes(m.payload)) {
          await updateOnlineUsers();
        }
        if (m.payload === "endtyping" && m.user_id !== uid) {
          setPeerTyping(false);
        }
      }

      if (m.event === EVENT_EDIT) {
        const pipeIdx = m.payload.indexOf("|");
        if (pipeIdx !== -1) {
          const targetMsgId = m.payload.substring(0, pipeIdx);
          const newPayload = m.payload.substring(pipeIdx + 1);
          setMessages((prev) =>
            prev.map((msg) =>
              msg.message_id === targetMsgId
                ? { ...msg, payload: newPayload, edited_at: m.time ? new Date(m.time).getTime() : Date.now() }
                : msg
            )
          );
        }
        return;
      }

      if (m.event === EVENT_DELETE) {
        const targetMsgId = m.payload;
        setMessages((prev) =>
          prev.map((msg) =>
            msg.message_id === targetMsgId
              ? { ...msg, deleted_for_all: true, deleted_by: m.user_id, payload: "" }
              : msg
          )
        );
        return;
      }

      if (m.event === EVENT_SEEN) {
        const seenMsgId = m.payload;
        setMessages((prev) =>
          prev.map((msg) =>
            msg.message_id === seenMsgId ? { ...msg, seen: true } : msg
          )
        );
        return;
      }

      if (m.event === EVENT_REACTION) {
        const parts = m.payload.split("|");
        if (parts.length === 3) {
          const [targetMsgId, emoji, action] = parts;
          setMessages((prev) =>
            prev.map((msg) => {
              if (msg.message_id !== targetMsgId) return msg;
              const reactions = [...(msg.reactions || [])];
              const idx = reactions.findIndex((r) => r.emoji === emoji);
              if (action === "add") {
                if (idx >= 0) {
                  if (!reactions[idx].user_ids.includes(m.user_id)) {
                    reactions[idx] = { ...reactions[idx], count: reactions[idx].count + 1, user_ids: [...reactions[idx].user_ids, m.user_id] };
                  }
                } else {
                  reactions.push({ emoji, count: 1, user_ids: [m.user_id] });
                }
              } else {
                if (idx >= 0) {
                  const userIdx = reactions[idx].user_ids.indexOf(m.user_id);
                  if (userIdx >= 0) {
                    reactions[idx] = { ...reactions[idx], count: reactions[idx].count - 1, user_ids: reactions[idx].user_ids.filter((_, i) => i !== userIdx) };
                    if (reactions[idx].count <= 0) reactions.splice(idx, 1);
                  }
                }
              }
              return { ...msg, reactions };
            })
          );
        }
        return;
      }

      if (m.event === EVENT_PIN) {
        const parts = m.payload.split("|");
        if (parts.length === 2) {
          const [targetMsgId, action] = parts;
          setMessages((prev) =>
            prev.map((msg) =>
              msg.message_id === targetMsgId ? { ...msg, is_pinned: action === "pin" } : msg
            )
          );
          if (action === "pin") {
            setPinnedMessages((prev) => [{ message_id: targetMsgId, pinned_by: m.user_id, pinned_at: Date.now() }, ...prev]);
          } else {
            setPinnedMessages((prev) => prev.filter((p) => p.message_id !== targetMsgId));
          }
        }
        return;
      }

      const processed = await processMessage(m);
      if (!processed) return;

      if (m.event === EVENT_TEXT && m.user_id !== uid) {
        setPeerTyping(false);
      }

      if ((m.event === EVENT_TEXT || m.event === EVENT_FILE) && m.user_id !== uid) {
        if (shouldNotifyIncomingMessage(m)) {
          sendBrowserNotification(notifPref === "mentions" ? "You were mentioned" : "You got a new message");
        }
      }

      if (m.user_id === uid && m.message_id) {
        setMessages((prev) => {
          const tempIdx = prev.findIndex(msg => msg.message_id?.startsWith('temp-'));
          if (tempIdx !== -1) {
            const newMsgs = [...prev];
            newMsgs[tempIdx] = { ...processed, status: "sent" as const };
            return newMsgs;
          }
          return mergeMessages(prev, [processed]);
        });
      } else {
        setMessages((prev) => mergeMessages(prev, [processed]));
      }
      void cacheChannelMessages([processed]);

      if ((m.event === EVENT_TEXT || m.event === EVENT_FILE) && m.user_id !== uid) {
        peerMessagesRef.current.push(m);
        ws.send({ event: EVENT_ACTION, user_id: uid, payload: `delivered:${m.message_id}` });
        if (!isPageHiddenRef.current) {
          markMessagesAsSeen();
        }
      }

      setTimeout(() => {
        if (chatContainerRef.current && isAtBottomRef.current) {
          chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
        }
      }, 50);
    },
    [processMessage, updateOnlineUsers, markMessagesAsSeen, mergeMessages, cacheChannelMessages, notifPref, shouldNotifyIncomingMessage]
  );

  const ws = useWebSocket({
    onMessage: handleWebSocketMessage,
    onStatusChange: (status) => {
      setConnectionStatus(status);
    },
  });

  const hasInitialized = useRef(false);

  useEffect(() => {
    useWebSocketRef.current = ws;
  }, [ws]);

  useEffect(() => {
    if (hasInitialized.current) return;

    const token = localStorage.getItem(ACCESS_TOKEN_KEY);
    const uid = localStorage.getItem("rc:userid");
    const username = localStorage.getItem("rc:username") || "User";
    const userpicture = localStorage.getItem("rc:userpicture") || "";
    setProfileName(username);
    setProfilePicture(userpicture);

    if (!token || !uid) {
      router.push("/");
      return;
    }
    setAccessToken(token);
    accessTokenRef.current = token;
    setUserId(uid);
    userIdRef.current = uid;

    peerMapRef.current.set(uid, {
      name: username,
      picture: userpicture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${uid}`,
    });

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const chatUrl = `${protocol}//${window.location.host}/api/chat?uid=${uid}&access_token=${token}`;
    ws.connect(chatUrl);
    hasInitialized.current = true;
  }, []);

  useEffect(() => {
    const handleVisibility = () => {
      isPageHiddenRef.current = document.visibilityState === "hidden";
      if (!isPageHiddenRef.current) {
        markMessagesAsSeen();
      }
    };
    document.addEventListener("visibilitychange", handleVisibility);
    return () => document.removeEventListener("visibilitychange", handleVisibility);
  }, [markMessagesAsSeen]);

  useEffect(() => {
    setNotificationPermission(getBrowserNotificationPermission());
  }, []);

  useEffect(() => {
    const handleBeforeUnload = () => {
      ws.disconnect();
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [ws]);

  const sendTextMessage = useCallback(() => {
    if (!draftText.trim()) return;

    const uid = userIdRef.current;
    const now = new Date();
    const timeStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}T${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`;
    const clientId = `temp-${Date.now()}`;

    const tempMsg: Msg = {
      event: EVENT_TEXT,
      user_id: uid,
      payload: draftText,
      message_id: clientId,
      time: timeStr,
      seen: false,
      side: "right",
      status: "sending",
      client_id: clientId,
    };
    setMessages((prev) => [...prev, tempMsg]);

    try {
      ws.send({ event: EVENT_TEXT, user_id: uid, payload: draftText, parent_id: replyTo?.messageId || "0" });
      clearDraft();
      setDraftText("");
      setReplyTo(null);
    } catch {
      setMessages((prev) =>
        prev.map((msg) =>
          msg.message_id === clientId ? { ...msg, status: "failed" as const } : msg
        )
      );
    }

    setTimeout(() => {
      if (chatContainerRef.current) {
        chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
      }
    }, 50);
  }, [ws, draftText, clearDraft]);

  const handleTyping = useCallback(() => {
    markMessagesAsSeen();
    if (typingTimeoutRef.current) clearTimeout(typingTimeoutRef.current);
    if (!isTypingRef.current) {
      ws.send({ event: EVENT_ACTION, user_id: userIdRef.current, payload: "istyping" });
      isTypingRef.current = true;
    }
    typingTimeoutRef.current = setTimeout(() => {
      ws.send({ event: EVENT_ACTION, user_id: userIdRef.current, payload: "endtyping" });
      isTypingRef.current = false;
    }, 1000);
  }, [ws, markMessagesAsSeen]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && !e.shiftKey && !mobileCheck()) {
        e.preventDefault();
        sendTextMessage();
      }
    },
    [sendTextMessage]
  );

  const handleRewriteDraft = useCallback(
    async (tone: string) => {
      const text = draftText.trim();
      if (!text || isAIWorking) return;
      setIsAIWorking(true);
      setAIError("");
      try {
        const result = await rewriteWithAI(text, tone);
        setDraftText(result.text);
        saveDraft(result.text);
      } catch {
        setAIError("AI rewrite failed");
      } finally {
        setIsAIWorking(false);
      }
    },
    [draftText, isAIWorking, saveDraft]
  );

  const handleFileUpload = useCallback(
    async (files: FileList | File[]) => {
      const fileArr = Array.from(files);
      const newUploads = fileArr.map((f) => ({
        id: `upload-${Date.now()}-${Math.random().toString(36).slice(2)}`,
        name: f.name,
        progress: 0,
        status: "uploading" as const,
      }));
      setUploads((prev) => [...prev, ...newUploads]);

      for (let i = 0; i < fileArr.length; i++) {
        const file = fileArr[i];
        const uploadId = newUploads[i].id;
        try {
          const objectKey = await chunkedUpload(file, (progress) => {
            setUploads((prev) => prev.map((u) => u.id === uploadId ? { ...u, progress } : u));
          });
          const payload: FilePayload = {
            file_name: file.name,
            object_key: objectKey,
          };
          ws.send({
            event: EVENT_FILE,
            user_id: userIdRef.current,
            payload: JSON.stringify(payload),
          });
          setUploads((prev) => prev.map((u) => u.id === uploadId ? { ...u, progress: 100, status: "done" } : u));
        } catch (err) {
          console.error("Upload error:", err);
          setUploads((prev) => prev.map((u) => u.id === uploadId ? { ...u, status: "error" } : u));
        }
      }

      setTimeout(() => {
        setUploads((prev) => prev.filter((u) => u.status === "uploading"));
      }, 2000);
    },
    [ws]
  );

  const handlePaste = useCallback(
    (e: React.ClipboardEvent) => {
      const items = e.clipboardData.items;
      const files: File[] = [];
      for (let i = 0; i < items.length; i++) {
        if (items[i].kind === "file") {
          const file = items[i].getAsFile();
          if (file) files.push(file);
        }
      }
      if (files.length > 0) handleFileUpload(files);
    },
    [handleFileUpload]
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.currentTarget === e.target) {
      setIsDragging(false);
    }
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);
      const files = e.dataTransfer.files;
      if (files.length > 0) handleFileUpload(files);
    },
    [handleFileUpload]
  );

  const handleLeave = useCallback(async () => {
    if (!confirm("Are you sure you want to leave?")) return;
    try {
      if (userIdRef.current) {
        await deleteChannel(userIdRef.current);
      }
    } catch (err) {
      console.error("Leave channel failed:", err);
    } finally {
      clearDraft();
      try {
        await clearAll();
      } catch (err) {
        console.error("Clear offline cache failed:", err);
      }
      localStorage.removeItem(ACCESS_TOKEN_KEY);
      setAccessToken("");
      accessTokenRef.current = "";
      setConnectionStatus("disconnected");
      ws.disconnect();
      router.replace("/");
    }
  }, [clearDraft, router, ws]);

  const loadMoreMessages = useCallback(async () => {
    if (isLoadingMore || !hasMoreMessages) return;
    setIsLoadingMore(true);
    try {
      const result = await fetchChannelMessages(pageState || undefined);
      const processed: Msg[] = [];
      for (let i = result.messages.length - 1; i >= 0; i--) {
        const m = result.messages[i];
        const p = await processMessage(m);
        if (p) {
          processed.push(p);
          if ((m.event === EVENT_TEXT || m.event === EVENT_FILE) && m.user_id !== userIdRef.current) {
            peerMessagesRef.current.unshift(m);
          }
        }
      }
      if (processed.length > 0) {
        if (chatContainerRef.current) {
          prevScrollHeightRef.current = chatContainerRef.current.scrollHeight;
        }
        setMessages((prev) => [...processed, ...prev]);
        if (result.next_ps && result.next_ps !== "") {
          setPageState(result.next_ps);
        } else {
          setHasMoreMessages(false);
        }
      } else {
        setHasMoreMessages(false);
      }
    } catch {
    } finally {
      setIsLoadingMore(false);
    }
  }, [isLoadingMore, hasMoreMessages, pageState, processMessage]);

  const loadInitialMessages = useCallback(async () => {
    const cacheKey = getChannelCacheKey();
    if (cacheKey) {
      try {
        const cached = await getCachedMessages(cacheKey);
        const processedCached: Msg[] = [];
        for (const m of cached) {
          const p = await processMessage(m);
          if (p) processedCached.push(p);
        }
        if (processedCached.length > 0) {
          setMessages((prev) => mergeMessages(prev, processedCached));
        }
      } catch (err) {
        console.error("Load cached messages failed:", err);
      }
    }

    try {
      let nextPageState: string | undefined;
      const fetched: Message[] = [];
      do {
        const result = await fetchChannelMessages(nextPageState);
        fetched.push(...result.messages);
        nextPageState = result.next_ps || undefined;
      } while (nextPageState);

      const processed: Msg[] = [];
      for (let i = fetched.length - 1; i >= 0; i--) {
        const m = fetched[i];
        const p = await processMessage(m);
        if (p) {
          processed.push(p);
          if ((m.event === EVENT_TEXT || m.event === EVENT_FILE) && m.user_id !== userIdRef.current) {
            peerMessagesRef.current.push(m);
          }
        }
      }
      setMessages((prev) => mergeMessages(prev, processed));
      await cacheChannelMessages(processed);
      setPageState("");
      setHasMoreMessages(false);
    } catch {
    }
  }, [processMessage, getChannelCacheKey, mergeMessages, cacheChannelMessages]);

  useEffect(() => {
    if (connectionStatus === "connected") {
      const savedDraft = loadDraft();
      if (savedDraft) {
        setDraftText(savedDraft);
      }
      (async () => {
        await loadInitialMessages();
        await fetchChannelUsers().then(async (result) => {
          for (const uid of result.user_ids) {
            if (uid !== userIdRef.current) {
              await getPeerInfo(uid);
            }
          }
          await updateOnlineUsers();
        });
        try {
          const pins = await fetchPinnedMessages();
          setPinnedMessages(pins);
          setMessages((prev) =>
            prev.map((msg) => ({
              ...msg,
              is_pinned: pins.some((p) => p.message_id === msg.message_id),
            }))
          );
        } catch {}
        try {
          const roleResult = await fetchMyRole(userIdRef.current);
          setMyRole(roleResult.role);
        } catch {}
        try {
          const notifResult = await fetchNotificationPrefs(userIdRef.current);
          setNotifPref(notifResult.pref);
        } catch {}
        setTimeout(() => {
          if (chatContainerRef.current) {
            chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
          }
        }, 100);
      })();
    }
  }, [connectionStatus, loadInitialMessages, getPeerInfo, updateOnlineUsers]);

  const handleEditMessage = useCallback(
    (messageId: string, newPayload: string) => {
      ws.send({ event: EVENT_EDIT, user_id: userIdRef.current, payload: `${messageId}|${newPayload}` });
      setMessages((prev) =>
        prev.map((msg) =>
          msg.message_id === messageId
            ? { ...msg, payload: newPayload, edited_at: Date.now() }
            : msg
        )
      );
    },
    [ws]
  );

  const handleDeleteMessage = useCallback(
    (messageId: string) => {
      ws.send({ event: EVENT_DELETE, user_id: userIdRef.current, payload: messageId });
      setMessages((prev) =>
        prev.map((msg) =>
          msg.message_id === messageId
            ? { ...msg, deleted_for_all: true, deleted_by: userIdRef.current, payload: "" }
            : msg
        )
      );
    },
    [ws]
  );

  const handleRetryMessage = useCallback(
    (clientId: string, payload: string, event: number) => {
      const uid = userIdRef.current;
      setMessages((prev) =>
        prev.map((msg) =>
          msg.message_id === clientId ? { ...msg, status: "sending" as const } : msg
        )
      );
      try {
        ws.send({ event, user_id: uid, payload });
      } catch {
        setMessages((prev) =>
          prev.map((msg) =>
            msg.message_id === clientId ? { ...msg, status: "failed" as const } : msg
          )
        );
      }
    },
    [ws]
  );

  const handleReaction = useCallback(
    (messageId: string, emoji: string, action: "add" | "remove") => {
      ws.send({ event: EVENT_REACTION, user_id: userIdRef.current, payload: `${messageId}|${emoji}|${action}` });
      setMessages((prev) =>
        prev.map((msg) => {
          if (msg.message_id !== messageId) return msg;
          const reactions = [...(msg.reactions || [])];
          const idx = reactions.findIndex((r) => r.emoji === emoji);
          if (action === "add") {
            if (idx >= 0) {
              if (!reactions[idx].user_ids.includes(userIdRef.current)) {
                reactions[idx] = { ...reactions[idx], count: reactions[idx].count + 1, user_ids: [...reactions[idx].user_ids, userIdRef.current] };
              }
            } else {
              reactions.push({ emoji, count: 1, user_ids: [userIdRef.current] });
            }
          } else {
            if (idx >= 0) {
              const userIdx = reactions[idx].user_ids.indexOf(userIdRef.current);
              if (userIdx >= 0) {
                reactions[idx] = { ...reactions[idx], count: reactions[idx].count - 1, user_ids: reactions[idx].user_ids.filter((_, i) => i !== userIdx) };
                if (reactions[idx].count <= 0) reactions.splice(idx, 1);
              }
            }
          }
          return { ...msg, reactions };
        })
      );
    },
    [ws]
  );

  const handlePin = useCallback(
    (messageId: string, action: "pin" | "unpin") => {
      ws.send({ event: EVENT_PIN, user_id: userIdRef.current, payload: `${messageId}|${action}` });
      setMessages((prev) =>
        prev.map((msg) =>
          msg.message_id === messageId ? { ...msg, is_pinned: action === "pin" } : msg
        )
      );
      if (action === "pin") {
        setPinnedMessages((prev) => [{ message_id: messageId, pinned_by: userIdRef.current, pinned_at: Date.now() }, ...prev]);
      } else {
        setPinnedMessages((prev) => prev.filter((p) => p.message_id !== messageId));
      }
    },
    [ws]
  );

  const handleReply = useCallback(
    (messageId: string, preview: string) => {
      setReplyTo({ messageId, preview });
      const input = document.getElementById("msg-input");
      if (input) input.focus();
    },
    []
  );

  const handleSearch = useCallback(async () => {
    if (!searchQuery.trim()) return;
    setIsSearching(true);
    try {
      const result = await searchMessages(searchQuery);
      const processed: Msg[] = [];
      for (const m of result.messages) {
        const side = m.user_id === userIdRef.current ? "right" : "left";
        processed.push({ ...m, side });
      }
      setSearchResults(processed);
    } catch {}
    setIsSearching(false);
  }, [searchQuery]);

  const handleOpenGallery = useCallback(async (type: string) => {
    setGalleryType(type);
    setShowGallery(true);
    setShowSearch(false);
    try {
      const result = await fetchMediaMessages(type);
      const processed: Msg[] = [];
      for (const m of result.messages) {
        const side = m.user_id === userIdRef.current ? "right" : "left";
        processed.push({ ...m, side });
      }
      setGalleryMessages(processed);
    } catch {}
  }, []);

  const handleNotificationPreferenceSelect = useCallback(async (value: string) => {
    setNotifPref(value);
    if (value !== "mute") {
      const permission = await requestBrowserNotificationPermission();
      setNotificationPermission(permission);
    } else {
      setNotificationPermission(getBrowserNotificationPermission());
    }

    try {
      await setNotificationPrefs(userIdRef.current, value);
    } catch {}
    setShowNotifSettings(false);
  }, []);

  const handleAvatarUpload = useCallback(async (file: File) => {
    setProfileError("");
    if (!file.type.startsWith("image/")) {
      setProfileError("Please choose an image file.");
      return;
    }
    try {
      const dataUrl = await resizeAvatarFile(file);
      setProfilePicture(dataUrl);
    } catch {
      setProfileError("Could not read this image.");
    }
  }, []);

  const handleSaveProfile = useCallback(async () => {
    const name = profileName.trim();
    if (!name || name.length > 30) {
      setProfileError("Name must be 1-30 characters.");
      return;
    }
    setIsSavingProfile(true);
    setProfileError("");
    try {
      const updated = await updateUserProfile(name, profilePicture);
      localStorage.setItem("rc:username", updated.name);
      localStorage.setItem("rc:userpicture", updated.picture || "");
      setProfileName(updated.name);
      setProfilePicture(updated.picture || "");
      if (userIdRef.current) {
        peerMapRef.current.set(userIdRef.current, {
          name: updated.name,
          picture: updated.picture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${userIdRef.current}`,
        });
      }
      setShowProfileSettings(false);
    } catch {
      setProfileError("Could not save profile.");
    } finally {
      setIsSavingProfile(false);
    }
  }, [profileName, profilePicture]);

  const handleScroll = useCallback(() => {
    if (!chatContainerRef.current) return;
    const el = chatContainerRef.current;
    const atBottom = el.scrollHeight - el.offsetHeight - el.scrollTop < 80;
    isAtBottomRef.current = atBottom;
    if (!isPageHiddenRef.current && atBottom) {
      markMessagesAsSeen();
    }
    if (el.scrollTop < 120 && hasMoreMessages && !isLoadingMore) {
      prevScrollHeightRef.current = el.scrollHeight;
      loadMoreMessages();
    }
  }, [hasMoreMessages, isLoadingMore, loadMoreMessages, markMessagesAsSeen]);

  useEffect(() => {
    if (prevScrollHeightRef.current > 0 && chatContainerRef.current) {
      const newScrollHeight = chatContainerRef.current.scrollHeight;
      const diff = newScrollHeight - prevScrollHeightRef.current;
      chatContainerRef.current.scrollTop += diff;
      prevScrollHeightRef.current = 0;
    }
  }, [messages]);

  const statusColor =
    connectionStatus === "connected"
      ? "text-emerald-400"
      : connectionStatus === "connecting"
      ? "text-amber-400"
      : "text-red-400";

  const StatusIcon = connectionStatus === "connected" ? Wifi : WifiOff;

  return (
    <div className="h-screen w-screen flex flex-col relative overflow-hidden">
      <AnimatedBackground />

      <motion.section
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.3 }}
        className="relative z-10 flex flex-col w-full h-full"
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <header className="flex justify-between items-center px-4 py-3 glass-dark border-b border-white/5">
          <div className="flex items-center gap-3">
            <motion.button
              whileHover={{ scale: 1.03 }}
              whileTap={{ scale: 0.97 }}
              onClick={() => {
                setShowProfileSettings(true);
                setShowNotifSettings(false);
              }}
              className="flex items-center gap-3 bg-transparent border-none p-0 cursor-pointer text-left"
            >
              <div
                className="w-9 h-9 rounded-xl bg-gradient-to-br from-accent-cyan/30 to-accent-violet/30 border border-white/10 bg-cover bg-center bg-no-repeat flex-shrink-0"
                style={{
                  backgroundImage: `url(${profilePicture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${userId || "user"}`})`,
                }}
              />
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold text-text-primary truncate max-w-[150px]">
                    {profileName || "User"}
                  </span>
                  {myRole !== "member" && (
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-md font-medium ${
                      myRole === "owner" ? "bg-amber-500/20 text-amber-400" : "bg-accent-violet/20 text-accent-violet"
                    }`}>
                      {myRole}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-1.5">
                  <StatusIcon className={`w-3 h-3 ${statusColor}`} />
                  <span className="text-xs text-text-muted capitalize">
                    {connectionStatus}
                  </span>
                </div>
              </div>
            </motion.button>
            <div className="hidden md:flex items-center gap-2 text-xs text-text-muted min-w-0">
              <Users className="w-3.5 h-3.5 text-text-secondary flex-shrink-0" />
              <span className="truncate max-w-[220px]">{onlineUsers}</span>
            </div>
          </div>

          <div className="flex items-center gap-1 sm:gap-2">
            <motion.button
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.95 }}
              onClick={() => { setShowNotifSettings(!showNotifSettings); }}
              className="flex items-center gap-1.5 px-2 sm:px-3 py-1.5 rounded-xl glass hover:bg-white/10 transition-all cursor-pointer text-sm"
            >
              {notifPref === "mute" ? (
                <BellOff className="w-3.5 h-3.5 text-red-400" />
              ) : notifPref === "mentions" ? (
                <AtSign className="w-3.5 h-3.5 text-amber-400" />
              ) : (
                <Bell className="w-3.5 h-3.5 text-emerald-400" />
              )}
            </motion.button>
            <motion.button
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.95 }}
              onClick={() => { setShowSearch(!showSearch); setShowGallery(false); }}
              className="flex items-center gap-1.5 px-2 sm:px-3 py-1.5 rounded-xl glass hover:bg-white/10 transition-all cursor-pointer text-sm"
            >
              <Search className="w-3.5 h-3.5 text-accent-cyan" />
            </motion.button>
            <motion.button
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.95 }}
              onClick={() => handleOpenGallery("all")}
              className="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-xl glass hover:bg-white/10 transition-all cursor-pointer text-sm"
            >
              <Image className="w-3.5 h-3.5 text-accent-pink" />
            </motion.button>
            <div className="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-xl glass">
              <MessageCircle className="w-3.5 h-3.5 text-accent-violet" />
              <span className="text-xs text-text-secondary font-medium">
                {messages.filter(m => m.event === EVENT_TEXT).length}
              </span>
            </div>
            <motion.button
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.95 }}
              onClick={handleLeave}
              className="flex items-center gap-1.5 px-2 sm:px-3 py-1.5 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 hover:bg-red-500/20 hover:border-red-500/30 transition-all cursor-pointer text-sm font-medium"
            >
              <LogOut className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Leave</span>
            </motion.button>
          </div>
        </header>

        {pinnedMessages.length > 0 && (
          <div className="px-4 py-2 bg-amber-500/5 border-b border-amber-500/10 flex items-center gap-2">
            <Pin className="w-3.5 h-3.5 text-amber-400 flex-shrink-0" />
            <span className="text-xs text-amber-400 font-medium">{pinnedMessages.length} pinned</span>
            <div className="flex-1 overflow-x-auto flex gap-2 scrollbar-none">
              {pinnedMessages.slice(0, 5).map((p) => {
                const pinnedMsg = messages.find((m) => m.message_id === p.message_id);
                if (!pinnedMsg) return null;
                return (
                  <button
                    key={p.message_id}
                    onClick={() => {
                      const el = chatContainerRef.current;
                      if (!el) return;
                      const msgEls = el.querySelectorAll("[data-message-id]");
                      msgEls.forEach((msgEl) => {
                        if (msgEl.getAttribute("data-message-id") === p.message_id) {
                          msgEl.scrollIntoView({ behavior: "smooth", block: "center" });
                        }
                      });
                    }}
                    className="flex-shrink-0 px-2 py-1 rounded-lg bg-white/5 text-[11px] text-text-secondary hover:bg-white/10 transition-colors cursor-pointer border-none max-w-[200px] truncate"
                  >
                    {pinnedMsg.deleted_for_all ? "Deleted message" : pinnedMsg.payload.slice(0, 40)}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        <div ref={containerRef} className="flex-1 relative overflow-hidden">
          {messages.length === 0 && connectionStatus === "connected" && !isLoadingMore && (
            <motion.div
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              className="flex flex-col items-center justify-center h-full gap-3 text-center absolute inset-0"
            >
              <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-accent-cyan/20 to-accent-violet/20 flex items-center justify-center border border-white/10">
                <MessageCircle className="w-8 h-8 text-accent-cyan" />
              </div>
              <p className="text-text-secondary font-medium">Matched! Say hello</p>
              <p className="text-text-muted text-sm">Start a conversation</p>
            </motion.div>
          )}

          <div
            ref={chatContainerRef}
            className="h-full overflow-y-auto scroll-smooth"
            onScroll={handleScroll}
          >
            <div className="min-h-full flex flex-col justify-end p-4">
              {messages.map((msg, idx) => (
                <ChatMessage
                  key={msg.message_id || idx}
                  message={msg}
                  userId={userId}
                  peerMap={peerMapRef.current}
                  accessToken={accessToken}
                  onImageClick={setModalImage}
                  onEdit={handleEditMessage}
                  onDelete={handleDeleteMessage}
                  onRetry={handleRetryMessage}
                  onReaction={handleReaction}
                  onPin={handlePin}
                  onReply={handleReply}
                />
              ))}

              {peerTyping && <TypingIndicator />}
            </div>
          </div>

          {isLoadingMore && (
            <div className="absolute top-4 left-1/2 -translate-x-1/2 z-10">
              <div className="flex items-center gap-2 px-4 py-2 rounded-xl glass">
                <Loader2 className="w-4 h-4 text-accent-violet animate-spin" />
                <span className="text-xs text-text-muted">Loading older messages...</span>
              </div>
            </div>
          )}
        </div>

        <ImageModal src={modalImage} onClose={() => setModalImage(null)} />

        {showProfileSettings && (
          <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
            <div className="absolute inset-0 bg-black/70 backdrop-blur-lg" onClick={() => setShowProfileSettings(false)} />
            <motion.div
              initial={{ opacity: 0, scale: 0.95, y: 12 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              className="relative z-[1] w-full max-w-sm glass-strong rounded-2xl border border-white/10 p-5 shadow-2xl shadow-black/40"
            >
              <div className="flex items-center justify-between mb-5">
                <h3 className="text-base font-semibold text-text-primary">Edit profile</h3>
                <button
                  onClick={() => setShowProfileSettings(false)}
                  className="w-8 h-8 rounded-lg bg-transparent hover:bg-white/10 border-none cursor-pointer flex items-center justify-center"
                >
                  <X className="w-4 h-4 text-text-muted" />
                </button>
              </div>

              <div className="flex flex-col items-center gap-4">
                <label className="relative cursor-pointer group">
                  <input
                    type="file"
                    accept="image/*"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) void handleAvatarUpload(file);
                      e.target.value = "";
                    }}
                  />
                  <div
                    className="w-24 h-24 rounded-2xl bg-gradient-to-br from-accent-cyan/30 to-accent-violet/30 border border-white/10 bg-cover bg-center bg-no-repeat"
                    style={{
                      backgroundImage: `url(${profilePicture || `https://api.dicebear.com/7.x/pixel-art/svg?seed=${userId || "user"}`})`,
                    }}
                  />
                  <div className="absolute inset-0 rounded-2xl bg-black/45 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                    <Camera className="w-6 h-6 text-white" />
                  </div>
                </label>

                <div className="w-full">
                  <label className="text-xs text-text-muted font-medium">Display name</label>
                  <input
                    value={profileName}
                    onChange={(e) => {
                      setProfileName(e.target.value);
                      setProfileError("");
                    }}
                    maxLength={30}
                    className="mt-2 w-full bg-white/5 rounded-xl px-4 py-2.5 border border-white/10 text-text-primary text-sm outline-none placeholder:text-text-muted focus:border-accent-violet/40 transition-colors"
                    placeholder="Enter your name"
                  />
                </div>

                {profileError && (
                  <div className="w-full px-3 py-2 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-xs">
                    {profileError}
                  </div>
                )}

                <div className="flex items-center gap-2 w-full">
                  <button
                    onClick={() => setShowProfileSettings(false)}
                    className="flex-1 px-4 py-2.5 rounded-xl bg-white/5 hover:bg-white/10 text-text-secondary border border-white/10 cursor-pointer transition-colors text-sm"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleSaveProfile}
                    disabled={isSavingProfile}
                    className="flex-1 px-4 py-2.5 rounded-xl bg-gradient-to-r from-accent-cyan to-accent-violet text-white border-none cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed transition-opacity text-sm font-medium flex items-center justify-center gap-2"
                  >
                    <Save className="w-4 h-4" />
                    {isSavingProfile ? "Saving" : "Save"}
                  </button>
                </div>
              </div>
            </motion.div>
          </div>
        )}

        <div className="px-3 sm:px-4 py-3 sm:py-4 glass-dark border-t border-white/5">
          {replyTo && (
            <div className="flex items-center gap-2 mb-2 px-3 py-2 rounded-xl bg-accent-violet/10 border border-accent-violet/20">
              <MessageSquare className="w-3.5 h-3.5 text-accent-violet flex-shrink-0" />
              <span className="text-xs text-text-secondary truncate flex-1">Replying to: {replyTo.preview}</span>
              <button
                onClick={() => setReplyTo(null)}
                className="text-text-muted hover:text-text-primary cursor-pointer bg-transparent border-none p-0"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          )}
          {(showAIPanel || aiError) && (
            <div className="mb-2 rounded-xl bg-white/5 border border-white/10 p-2">
              {showAIPanel && (
                <div className="flex items-center gap-2 overflow-x-auto pb-1">
                  {[
                    { label: "Professional", tone: "professional and concise" },
                    { label: "Friendly", tone: "friendly and natural" },
                    { label: "Shorter", tone: "shorter and clearer" },
                  ].map((item) => (
                    <button
                      key={item.label}
                      onClick={() => handleRewriteDraft(item.tone)}
                      disabled={!draftText.trim() || isAIWorking}
                      title={item.label}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-text-secondary hover:text-text-primary border border-white/10 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed transition-colors text-xs whitespace-nowrap"
                    >
                      <Sparkles className="w-3.5 h-3.5 text-accent-cyan" />
                      {item.label}
                    </button>
                  ))}
                  {isAIWorking && <Loader2 className="w-4 h-4 text-accent-violet animate-spin flex-shrink-0" />}
                </div>
              )}
              {aiError && (
                <div className="mt-1 px-2 py-1 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-xs">
                  {aiError}
                </div>
              )}
            </div>
          )}
          <div className="flex items-end gap-2 sm:gap-3">
            <label className="flex items-center justify-center w-9 h-9 sm:w-10 sm:h-10 rounded-xl glass hover:bg-white/10 transition-all cursor-pointer group flex-shrink-0 mb-0.5">
              <input
                type="file"
                multiple
                className="hidden"
                onChange={(e) => {
                  if (e.target.files) handleFileUpload(e.target.files);
                  e.target.value = "";
                }}
              />
              <Paperclip className="w-4 h-4 sm:w-4.5 sm:h-4.5 text-text-secondary group-hover:text-accent-violet transition-colors" />
            </label>
            <button
              onClick={() => setShowAIPanel((current) => !current)}
              disabled={connectionStatus !== "connected"}
              title="AI"
              className={`flex items-center justify-center w-9 h-9 sm:w-10 sm:h-10 rounded-xl glass hover:bg-white/10 transition-all cursor-pointer border border-white/10 flex-shrink-0 mb-0.5 disabled:opacity-50 disabled:cursor-not-allowed ${
                showAIPanel ? "bg-white/10" : ""
              }`}
            >
              {isAIWorking ? (
                <Loader2 className="w-4 h-4 sm:w-4.5 sm:h-4.5 text-accent-violet animate-spin" />
              ) : (
                <Sparkles className="w-4 h-4 sm:w-4.5 sm:h-4.5 text-accent-cyan" />
              )}
            </button>

            <div className="flex-1 relative group">
              <div className="absolute inset-0 bg-gradient-to-r from-accent-cyan/10 via-accent-violet/10 to-accent-pink/10 rounded-2xl blur-lg opacity-0 group-focus-within:opacity-100 transition-opacity duration-500" />
              <textarea
                id="msg-input"
                placeholder="Type a message..."
                rows={1}
                value={draftText}
                onChange={(e) => {
                  setDraftText(e.target.value);
                  saveDraft(e.target.value);
                }}
                disabled={connectionStatus !== "connected"}
                className="relative w-full bg-white/5 rounded-2xl resize-none px-4 sm:px-5 py-2.5 sm:py-3 border border-white/10 text-text-primary text-sm outline-none placeholder:text-text-muted focus:border-accent-violet/40 transition-all duration-300 font-[Inter,sans-serif] disabled:opacity-50"
                onKeyDown={handleKeyDown}
                onKeyUp={handleTyping}
                onPaste={handlePaste}
                style={{ maxHeight: "15rem" }}
                onInput={(e) => {
                  const el = e.currentTarget;
                  el.style.height = "auto";
                  el.style.height = el.scrollHeight + "px";
                }}
              />
            </div>

            <motion.button
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.9 }}
              onClick={sendTextMessage}
              disabled={connectionStatus !== "connected"}
              className="flex items-center justify-center w-9 h-9 sm:w-10 sm:h-10 rounded-xl bg-gradient-to-r from-accent-cyan to-accent-violet cursor-pointer border-none shadow-lg shadow-accent-violet/20 hover:shadow-accent-violet/40 transition-shadow disabled:opacity-50 disabled:cursor-not-allowed mb-0.5 flex-shrink-0"
            >
              <Send className="w-4 h-4 sm:w-4.5 sm:h-4.5 text-white" />
            </motion.button>
          </div>
        </div>

        {isDragging && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm border-2 border-dashed border-accent-violet/60">
            <div className="flex flex-col items-center gap-3">
              <Upload className="w-12 h-12 text-accent-violet animate-bounce" />
              <p className="text-text-primary font-semibold text-lg">Drop files to upload</p>
            </div>
          </div>
        )}

        {uploads.length > 0 && (
          <div className="absolute bottom-20 sm:bottom-24 right-3 sm:right-6 z-40 flex flex-col gap-2 w-56 sm:w-64">
            {uploads.map((u) => (
              <div key={u.id} className="glass-strong rounded-xl p-3 border border-white/10 shadow-lg">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-xs text-text-primary font-medium truncate max-w-[180px]">{u.name}</span>
                  {u.status === "done" ? (
                    <span className="text-[10px] text-emerald-400">Done</span>
                  ) : u.status === "error" ? (
                    <span className="text-[10px] text-red-400">Failed</span>
                  ) : (
                    <span className="text-[10px] text-text-muted">{u.progress}%</span>
                  )}
                </div>
                <div className="h-1.5 rounded-full bg-white/10 overflow-hidden">
                  <div
                    className={`h-full rounded-full transition-all duration-300 ${
                      u.status === "error" ? "bg-red-500" : u.status === "done" ? "bg-emerald-500" : "bg-gradient-to-r from-accent-cyan to-accent-violet"
                    }`}
                    style={{ width: `${u.progress}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}

        {showSearch && (
          <div className="fixed inset-0 z-50 glass-strong flex flex-col">
            <div className="flex items-center gap-3 px-4 py-3 border-b border-white/5">
              <Search className="w-4 h-4 text-text-muted flex-shrink-0" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                placeholder="Search messages..."
                className="flex-1 bg-transparent border-none outline-none text-text-primary text-sm placeholder:text-text-muted"
                autoFocus
              />
              <button onClick={() => { setShowSearch(false); setSearchQuery(""); setSearchResults([]); }} className="cursor-pointer bg-transparent border-none p-1 hover:bg-white/10 rounded-lg transition-colors">
                <X className="w-4 h-4 text-text-muted" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4">
              {isSearching && <div className="text-center text-text-muted text-sm mt-8">Searching...</div>}
              {!isSearching && searchResults.length === 0 && searchQuery && (
                <div className="text-center text-text-muted text-sm mt-8">No results found</div>
              )}
              {!isSearching && !searchQuery && (
                <div className="text-center text-text-muted text-sm mt-8">Enter a keyword to search</div>
              )}
              {searchResults.map((msg, idx) => (
                <ChatMessage
                  key={msg.message_id || idx}
                  message={msg}
                  userId={userId}
                  peerMap={peerMapRef.current}
                  accessToken={accessToken}
                  onImageClick={setModalImage}
                />
              ))}
            </div>
          </div>
        )}

        {showGallery && (
          <div className="fixed inset-0 z-50 glass-strong flex flex-col">
            <div className="flex items-center gap-2 px-4 py-3 border-b border-white/5 flex-wrap">
              {["all", "image", "video", "audio", "document"].map((type) => (
                <button
                  key={type}
                  onClick={() => handleOpenGallery(type)}
                  className={`px-3 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer border-none ${
                    galleryType === type ? "bg-accent-violet/20 text-accent-violet" : "bg-white/5 text-text-muted hover:bg-white/10"
                  }`}
                >
                  {type.charAt(0).toUpperCase() + type.slice(1)}
                </button>
              ))}
              <div className="flex-1" />
              <button onClick={() => setShowGallery(false)} className="cursor-pointer bg-transparent border-none p-1 hover:bg-white/10 rounded-lg transition-colors">
                <X className="w-4 h-4 text-text-muted" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4">
              {galleryMessages.length === 0 && (
                <div className="text-center text-text-muted text-sm mt-8">No media found</div>
              )}
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
                {galleryMessages.map((msg) => (
                  <GalleryItem key={msg.message_id} message={msg} onImageClick={setModalImage} peerMap={peerMapRef.current} />
                ))}
              </div>
            </div>
          </div>
        )}

        {showNotifSettings && (
          <div className="absolute top-12 sm:top-14 right-2 sm:right-4 z-50 glass-strong rounded-xl border border-white/10 p-4 shadow-xl shadow-black/30 w-56">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-text-primary">Notifications</h3>
              <button onClick={() => setShowNotifSettings(false)} className="text-text-muted hover:text-text-primary cursor-pointer bg-transparent border-none p-0">
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
            <div className="mb-3 px-3 py-2 rounded-lg bg-white/5">
              <span className="text-[11px] text-text-muted">
                {getNotificationPermissionLabel(notificationPermission)}
              </span>
            </div>
            {[
              { value: "all", label: "All messages", icon: Bell, color: "text-emerald-400" },
              { value: "mentions", label: "Mentions only", icon: AtSign, color: "text-amber-400" },
              { value: "mute", label: "Muted", icon: BellOff, color: "text-red-400" },
            ].map(({ value, label, icon: Icon, color }) => (
              <button
                key={value}
                onClick={() => handleNotificationPreferenceSelect(value)}
                className={`flex items-center gap-2.5 w-full px-3 py-2 rounded-lg text-sm transition-colors cursor-pointer border-none text-left ${
                  notifPref === value ? "bg-white/10" : "bg-transparent hover:bg-white/5"
                }`}
              >
                <Icon className={`w-4 h-4 ${color}`} />
                <span className="text-text-primary">{label}</span>
              </button>
            ))}
          </div>
        )}
      </motion.section>
    </div>
  );
}

function getBrowserNotificationPermission(): NotificationPermission | "unsupported" {
  if (typeof Notification === "undefined") return "unsupported";
  return Notification.permission;
}

function resizeAvatarFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("Failed to read image"));
    reader.onload = () => {
      const img = document.createElement("img");
      img.onerror = () => reject(new Error("Failed to load image"));
      img.onload = () => {
        const size = 256;
        const canvas = document.createElement("canvas");
        canvas.width = size;
        canvas.height = size;
        const ctx = canvas.getContext("2d");
        if (!ctx) {
          reject(new Error("Canvas not available"));
          return;
        }

        const minSide = Math.min(img.width, img.height);
        const sx = (img.width - minSide) / 2;
        const sy = (img.height - minSide) / 2;
        ctx.drawImage(img, sx, sy, minSide, minSide, 0, 0, size, size);
        resolve(canvas.toDataURL("image/jpeg", 0.85));
      };
      img.src = String(reader.result || "");
    };
    reader.readAsDataURL(file);
  });
}

async function requestBrowserNotificationPermission(): Promise<NotificationPermission | "unsupported"> {
  if (typeof Notification === "undefined") return "unsupported";
  if (Notification.permission === "default") {
    return Notification.requestPermission();
  }
  return Notification.permission;
}

function getNotificationPermissionLabel(permission: NotificationPermission | "unsupported"): string {
  switch (permission) {
    case "granted":
      return "Browser notifications allowed";
    case "denied":
      return "Browser notifications blocked";
    case "unsupported":
      return "Browser notifications unsupported";
    default:
      return "Browser notifications not allowed yet";
  }
}

function sendBrowserNotification(msg: string) {
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  new Notification("NexusChat", { body: msg });
}
