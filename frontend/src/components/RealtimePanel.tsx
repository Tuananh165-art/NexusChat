"use client";

import { useEffect, useRef, useState } from "react";
import {
  Bell,
  Check,
  Phone,
  PhoneCall,
  PhoneOff,
  Video,
  X,
} from "lucide-react";
import {
  createCall,
  fetchIceConfig,
  fetchNotificationUnreadCount,
  fetchNotifications,
  fetchPushPublicKey,
  markAllNotificationsRead,
  markNotificationRead,
  savePushSubscription,
  type RealtimeNotification,
} from "@/lib/api";

type Props = { userId: string; accessToken: string };
type CallSignal = { type: string; call_id: string; target_user_id?: string; payload?: unknown; };

function base64ToBytes(value: string) {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - normalized.length % 4) % 4);
  return Uint8Array.from(atob(padded), (char) => char.charCodeAt(0));
}

export default function RealtimePanel({ userId, accessToken }: Props) {
  const [notifications, setNotifications] = useState<RealtimeNotification[]>([]);
  const [unread, setUnread] = useState(0);
  const [showNotifications, setShowNotifications] = useState(false);
  const [pushReady, setPushReady] = useState(false);
  const [calleeId, setCalleeId] = useState("");
  const [callState, setCallState] = useState("");
  const [incomingCall, setIncomingCall] = useState<{ call_id: string; caller_id: string; media: "audio" | "video" } | null>(null);
  const [error, setError] = useState("");
  const notificationWS = useRef<WebSocket | null>(null);
  const presenceWS = useRef<WebSocket | null>(null);
  const callWS = useRef<WebSocket | null>(null);
  const callRef = useRef<{ id: string; peer: string; media: "audio" | "video" } | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const localVideo = useRef<HTMLVideoElement | null>(null);
  const remoteVideo = useRef<HTMLVideoElement | null>(null);

  const wsURL = (path: string) => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}${path}?uid=${encodeURIComponent(userId)}&access_token=${encodeURIComponent(accessToken)}`;
  };

  const sendSignal = (signal: CallSignal) => callWS.current?.readyState === WebSocket.OPEN && callWS.current.send(JSON.stringify(signal));

  const closePeer = () => {
    peerRef.current?.close();
    peerRef.current = null;
    callRef.current = null;
    setCallState("");
    if (localVideo.current) localVideo.current.srcObject = null;
    if (remoteVideo.current) remoteVideo.current.srcObject = null;
  };

  const startPeer = async (callId: string, peer: string, media: "audio" | "video", offer: boolean) => {
    const config = await fetchIceConfig(userId);
    const pc = new RTCPeerConnection({ iceServers: config.ice_servers || [] });
    peerRef.current = pc;
    callRef.current = { id: callId, peer, media };
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: media === "video" });
    stream.getTracks().forEach((track) => pc.addTrack(track, stream));
    if (localVideo.current) {
      localVideo.current.srcObject = stream;
      localVideo.current.muted = true;
    }
    pc.ontrack = (event) => {
      if (remoteVideo.current) remoteVideo.current.srcObject = event.streams[0];
    };
    pc.onicecandidate = (event) => {
      if (event.candidate) sendSignal({ type: "ice_candidate", call_id: callId, target_user_id: peer, payload: event.candidate });
    };
    if (offer) {
      const offerDescription = await pc.createOffer();
      await pc.setLocalDescription(offerDescription);
      sendSignal({ type: "offer", call_id: callId, target_user_id: peer, payload: offerDescription });
    }
  };

  useEffect(() => {
    if (!userId || !accessToken) return;
    void fetchNotifications(30, userId).then(setNotifications).catch(() => {});
    void fetchNotificationUnreadCount(userId).then(setUnread).catch(() => {});
    const presence = new WebSocket(wsURL("/api/presence/ws"));
    const notification = new WebSocket(wsURL("/api/notifications/ws"));
    const call = new WebSocket(wsURL("/api/calls/ws"));
    presenceWS.current = presence;
    notificationWS.current = notification;
    callWS.current = call;
    const heartbeat = window.setInterval(() => {
      if (presence.readyState === WebSocket.OPEN) presence.send(JSON.stringify({ type: "heartbeat" }));
    }, 15000);
    notification.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        if (message.type === "notification.created") {
          setNotifications((current) => [message.data, ...current].slice(0, 30));
          setUnread((count) => count + 1);
        }
      } catch {}
    };
    call.onmessage = (event) => {
      void (async () => {
        const signal = JSON.parse(event.data) as CallSignal & { data?: any };
        if (signal.type === "call.invite") {
          const data = signal.data;
          setIncomingCall({ call_id: data.id, caller_id: data.caller_id, media: data.media });
          return;
        }
        const callData = signal.data;
        const active = callRef.current;
        if (!active || active.id !== signal.call_id) return;
        if (signal.type === "call.accepted") {
          setCallState("accepted");
          return;
        }
        if (signal.type === "call.ended") {
          closePeer();
          return;
        }
        if (signal.type === "offer" && peerRef.current) {
          await peerRef.current.setRemoteDescription(signal.payload as RTCSessionDescriptionInit);
          const answer = await peerRef.current.createAnswer();
          await peerRef.current.setLocalDescription(answer);
          sendSignal({ type: "answer", call_id: signal.call_id, target_user_id: active.peer, payload: answer });
          return;
        }
        if (signal.type === "answer" && peerRef.current) {
          await peerRef.current.setRemoteDescription(signal.payload as RTCSessionDescriptionInit);
          sendSignal({ type: "connected", call_id: signal.call_id, target_user_id: active.peer });
        }
        if (signal.type === "ice_candidate" && peerRef.current && signal.payload) {
          await peerRef.current.addIceCandidate(signal.payload as RTCIceCandidateInit);
        }
        void callData;
      })().catch((err) => setError(err instanceof Error ? err.message : "Call failed"));
    };
    return () => {
      window.clearInterval(heartbeat);
      presence.close();
      notification.close();
      call.close();
      closePeer();
    };
  }, [userId, accessToken]);

  const makeCall = async () => {
    try {
      setError("");
      if (!calleeId.trim()) throw new Error("Enter the peer user id");
      const call = await createCall(calleeId.trim(), "video", userId);
      await startPeer(call.id, calleeId.trim(), "video", true);
      setCallState("ringing");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to start call");
    }
  };

  const acceptCall = async () => {
    if (!incomingCall) return;
    try {
      const call = incomingCall;
      setIncomingCall(null);
      sendSignal({ type: "accept", call_id: call.call_id, target_user_id: call.caller_id });
      await startPeer(call.call_id, call.caller_id, call.media, false);
      setCallState("accepted");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to accept call");
    }
  };

  const enablePush = async () => {
    try {
      if (!("serviceWorker" in navigator) || !("PushManager" in window)) throw new Error("Web Push is not supported");
      const registration = await navigator.serviceWorker.register("/sw.js");
      const key = await fetchPushPublicKey();
      if (!key) throw new Error("Web Push is not configured");
      const subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: base64ToBytes(key) });
      await savePushSubscription(subscription, userId);
      setPushReady(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Push setup failed");
    }
  };

  return (
    <div className="relative flex items-center gap-2">
      <div className="hidden md:flex items-center gap-1.5">
        <input value={calleeId} onChange={(event) => setCalleeId(event.target.value)} placeholder="Peer ID" className="w-20 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-[11px] text-text-primary" />
        <button onClick={() => void makeCall()} title="Start video call" className="rounded-lg bg-emerald-500/15 p-2 text-emerald-300 hover:bg-emerald-500/25"><Video className="h-4 w-4" /></button>
      </div>
      <button onClick={() => setShowNotifications((show) => !show)} title="Notifications" className="relative rounded-lg p-2 text-text-secondary hover:bg-white/10">
        <Bell className="h-4 w-4" />
        {unread > 0 && <span className="absolute -right-1 -top-1 rounded-full bg-accent-pink px-1.5 text-[9px] text-white">{unread > 99 ? "99+" : unread}</span>}
      </button>
      {showNotifications && (
        <div className="absolute right-0 top-10 z-50 w-80 rounded-xl border border-white/10 bg-slate-950/95 p-3 shadow-xl">
          <div className="mb-2 flex items-center justify-between"><span className="text-sm font-semibold text-white">Notifications</span><button onClick={() => void markAllNotificationsRead(userId).then(() => setUnread(0))} className="text-xs text-accent-cyan">Read all</button></div>
          <div className="max-h-64 space-y-1 overflow-y-auto">
            {notifications.length === 0 && <p className="py-4 text-center text-xs text-text-muted">No notifications</p>}
            {notifications.map((item) => <button key={item.id} onClick={() => void markNotificationRead(item.id, userId).then(() => setUnread((count) => Math.max(0, count - 1)))} className="flex w-full gap-2 rounded-lg p-2 text-left hover:bg-white/10"><span className="mt-0.5 text-accent-violet"><Check className="h-3.5 w-3.5" /></span><span><span className="block text-xs font-medium text-white">{item.title}</span><span className="block text-[11px] text-text-muted">{item.body}</span></span></button>)}
          </div>
          <button onClick={() => void enablePush()} className="mt-2 flex w-full items-center justify-center gap-1 rounded-lg bg-white/5 px-2 py-1.5 text-xs text-text-secondary hover:bg-white/10">{pushReady ? "Web Push enabled" : "Enable Web Push"}</button>
          {error && <p className="mt-2 text-[11px] text-red-400">{error}</p>}
        </div>
      )}
      {incomingCall && <div className="absolute right-0 top-10 z-50 w-64 rounded-xl border border-emerald-400/30 bg-slate-950/95 p-3 shadow-xl"><p className="text-sm text-white">Incoming {incomingCall.media} call</p><p className="mt-1 text-xs text-text-muted">From {incomingCall.caller_id}</p><div className="mt-3 flex gap-2"><button onClick={() => void acceptCall()} className="flex flex-1 items-center justify-center gap-1 rounded-lg bg-emerald-500/20 py-2 text-xs text-emerald-300"><PhoneCall className="h-3.5 w-3.5" />Accept</button><button onClick={() => { sendSignal({ type: "reject", call_id: incomingCall.call_id }); setIncomingCall(null); }} className="flex flex-1 items-center justify-center gap-1 rounded-lg bg-red-500/20 py-2 text-xs text-red-300"><PhoneOff className="h-3.5 w-3.5" />Reject</button></div></div>}
      {callState && <div className="fixed bottom-4 right-4 z-50 w-72 rounded-xl border border-accent-cyan/30 bg-slate-950/95 p-3 shadow-xl"><div className="mb-2 flex items-center justify-between"><span className="text-xs text-accent-cyan">{callState}</span><button onClick={() => { if (callRef.current) sendSignal({ type: "hangup", call_id: callRef.current.id }); closePeer(); }} className="rounded-lg bg-red-500/20 p-1.5 text-red-300"><X className="h-3.5 w-3.5" /></button></div><div className="grid grid-cols-2 gap-2"><video ref={localVideo} autoPlay playsInline className="aspect-video rounded-lg bg-black object-cover" /><video ref={remoteVideo} autoPlay playsInline className="aspect-video rounded-lg bg-black object-cover" /></div><div className="mt-2 flex items-center justify-center gap-2 text-text-muted"><Phone className="h-3.5 w-3.5" />P2P WebRTC</div></div>}
    </div>
  );
}
