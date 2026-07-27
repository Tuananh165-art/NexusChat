"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import { ArrowRight, Sparkles, Loader2, MessageCircle, UsersRound, Hash, ShieldCheck } from "lucide-react";
import { ACCESS_TOKEN_KEY, CONVERSATION_CONTEXT_KEY, type ConversationContext, type UserInfo } from "@/lib/constants";
import { fetchMe, joinRoom, openRoom, login, signup } from "@/lib/api";
import FriendRequestsPanel from "@/components/FriendRequestsPanel";
import AnimatedBackground from "@/components/AnimatedBackground";
import UserDiscoveryPanel from "@/components/UserDiscoveryPanel";

function saveUser(user: { id: string; username?: string; handle?: string; name: string; picture?: string }) {
  localStorage.setItem("rc:userid", user.id);
  localStorage.setItem("rc:username", user.name);
  localStorage.setItem("rc:userhandle", user.handle || "");
  localStorage.setItem("rc:userpicture", user.picture || "");
}

export default function HomePage() {
  const router = useRouter();
  const [step, setStep] = useState<"input" | "done" | "matching" | "search">("input");
  const [authMode, setAuthMode] = useState<"signup" | "login">("signup");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [authError, setAuthError] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [matchError, setMatchError] = useState("");
  const [inviteError, setInviteError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [currentUser, setCurrentUser] = useState<UserInfo | null>(null);
  const [authBusy, setAuthBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const oauthError = new URLSearchParams(window.location.search).get("oauth_error");
    if (oauthError) {
      const messages: Record<string, string> = {
        google_not_configured: "Google sign-in is not configured.",
        missing_oauth_state: "Your Google sign-in session expired. Please try again.",
        invalid_oauth_state: "The Google sign-in state is invalid. Please try again.",
        google_code_exchange_failed: "Google sign-in could not be completed.",
        google_profile_failed: "Google profile information could not be loaded.",
        google_account_failed: "Your NexusChat account could not be created.",
        session_failed: "Your sign-in session could not be created.",
      };
      setAuthError(messages[oauthError] || "Google sign-in failed. Please try again.");
      window.history.replaceState({}, "", window.location.pathname);
    }
    (async () => {
      try {
        const me = await fetchMe();
        setCurrentUser(me);
        saveUser(me);
        const token = localStorage.getItem(ACCESS_TOKEN_KEY);
        const skipAutoResume = localStorage.getItem("rc:skip-auto-resume") === "1";
        localStorage.removeItem("rc:skip-auto-resume");
        if (token && !skipAutoResume) router.push("/chat");
        else setStep("done");
      } catch {
        localStorage.removeItem(ACCESS_TOKEN_KEY);
        localStorage.removeItem("rc:userid");
        localStorage.removeItem("rc:username");
        localStorage.removeItem("rc:userhandle");
        localStorage.removeItem("rc:userpicture");
        setStep("input");
      } finally { setIsLoading(false); }
    })();
  }, [router]);

  useEffect(() => {
    if (step === "input" && !isLoading) inputRef.current?.focus();
  }, [step, isLoading, authMode]);

  const submitAuth = async () => {
    setAuthError("");
    if (!username.trim() || !password) { setAuthError("Enter your username and password."); return; }
    setAuthBusy(true);
    try {
      const result = authMode === "signup"
        ? await signup(username.trim(), password, displayName.trim())
        : await login(username.trim(), password);
      saveUser(result.user);
      setCurrentUser(result.user);
      setPassword("");
      setStep("done");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Sign-in failed";
      if (message.includes("already taken")) setAuthError("That username is already taken.");
      else if (message.includes("invalid username")) setAuthError("Use 3–30 lowercase letters, numbers, or underscores.");
      else if (message.includes("password")) setAuthError("Your password must contain at least 8 characters.");
      else if (message.includes("invalid username or password")) setAuthError("The username or password is incorrect.");
      else setAuthError(message);
    } finally { setAuthBusy(false); }
  };

  const openChat = (accessToken: string, channelId: string, context?: ConversationContext) => {
    if (!accessToken || !channelId) return;
    localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
    if (context) localStorage.setItem(CONVERSATION_CONTEXT_KEY, JSON.stringify(context));
    router.push("/chat");
  };

  const joinGroup = async () => {
    if (!inviteCode.trim()) return;
    setInviteError("");
    try {
      const room = await joinRoom(inviteCode.trim());
      const result = await openRoom(room.channel_id);
      openChat(result.access_token, result.channel_id, { channel_id: result.channel_id, kind: "group", title: room.name, avatar: room.avatar, member_count: room.member_count });
    } catch { setInviteError("That invite code is invalid or the room does not exist."); }
  };

  const startMatching = () => {
    setMatchError(""); setStep("matching");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/match`);
    wsRef.current = ws;
    const timeout = window.setTimeout(() => {
      if (wsRef.current !== ws) return;
      wsRef.current = null;
      ws.close();
      setMatchError("No match was found yet. Keep this window open or try again later.");
    }, 60000);
    ws.addEventListener("message", (event) => {
      let result: { channel_id?: string; access_token?: string; peer_user_id?: string; error?: string };
      try {
        result = JSON.parse(event.data) as typeof result;
      } catch {
        setMatchError("The matching service returned an invalid response.");
        return;
      }
      if (result.error) {
        window.clearTimeout(timeout);
        if (wsRef.current === ws) {
          wsRef.current = null;
          ws.close();
          setMatchError(result.error);
        }
        return;
      }
      if (result.channel_id && result.access_token) {
        window.clearTimeout(timeout);
        wsRef.current = null;
        ws.close();
        openChat(result.access_token, result.channel_id, { channel_id: result.channel_id, kind: "random", title: "Random match", peer_user_id: result.peer_user_id || "" });
      }
    });
    ws.addEventListener("error", () => {
      window.clearTimeout(timeout);
      if (wsRef.current === ws) setMatchError("Matching service could not be reached.");
    });
    ws.addEventListener("close", () => {
      window.clearTimeout(timeout);
      if (wsRef.current === ws) setMatchError("The matching connection was closed.");
    });
  };

  const cancelMatching = () => {
    if (wsRef.current) { const ws = wsRef.current; wsRef.current = null; ws.close(); }
    setMatchError(""); setStep("done");
  };

  useEffect(() => () => {
    if (wsRef.current) { const ws = wsRef.current; wsRef.current = null; ws.close(); }
  }, []);

  if (isLoading) return <div className="relative flex min-h-screen items-center justify-center"><AnimatedBackground /><Loader2 className="relative z-10 h-8 w-8 animate-spin text-accent-violet" /></div>;

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden px-4 py-8">
      <AnimatedBackground />
      <div className="relative z-10 flex w-full items-center justify-center">
        <AnimatePresence mode="wait">
          {step === "input" && (
            <motion.div key="input" initial={{ opacity: 0, y: 32 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -32 }} className="auth-card glass-strong w-full max-w-md rounded-[2rem] p-8 shadow-2xl shadow-black/30 sm:p-10">
              <div className="mb-8 text-center">
                <div className="mx-auto mb-5 flex h-16 w-16 items-center justify-center rounded-2xl border border-white/15 bg-gradient-to-br from-accent-cyan/25 to-accent-violet/25 shadow-lg shadow-accent-violet/20"><Sparkles className="h-8 w-8 text-accent-cyan" /></div>
                <p className="mb-2 text-xs font-semibold uppercase tracking-[0.3em] text-accent-cyan">Real-time conversations</p>
                <h1 className="mb-3 text-4xl font-bold tracking-tight text-gradient">NexusChat</h1>
                <p className="text-sm leading-6 text-text-secondary">Connect with people, keep your conversations, and chat safely across every device.</p>
              </div>
              <div className="mb-6 grid grid-cols-2 rounded-2xl border border-white/10 bg-black/20 p-1.5">
                <button onClick={() => { setAuthMode("signup"); setAuthError(""); }} className={`rounded-xl py-2.5 text-sm font-semibold transition ${authMode === "signup" ? "bg-white/12 text-white shadow-lg" : "text-text-secondary hover:text-white"}`}>Create account</button>
                <button onClick={() => { setAuthMode("login"); setAuthError(""); }} className={`rounded-xl py-2.5 text-sm font-semibold transition ${authMode === "login" ? "bg-white/12 text-white shadow-lg" : "text-text-secondary hover:text-white"}`}>Sign in</button>
              </div>
              <div className="space-y-3.5">
                <label className="block"><span className="mb-1.5 block text-xs font-medium text-text-secondary">Username</span><input ref={inputRef} value={username} onChange={(event) => setUsername(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void submitAuth(); }} placeholder="Choose a username" autoComplete="username" maxLength={30} className="w-full rounded-xl border border-white/10 bg-white/5 px-4 py-3.5 text-white outline-none transition placeholder:text-text-muted focus:border-accent-violet/70 focus:bg-white/10 focus:ring-4 focus:ring-accent-violet/10" /></label>
                {authMode === "signup" && <label className="block"><span className="mb-1.5 block text-xs font-medium text-text-secondary">Display name <span className="text-text-muted">(optional)</span></span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="How should people see you?" maxLength={30} className="w-full rounded-xl border border-white/10 bg-white/5 px-4 py-3.5 text-white outline-none transition placeholder:text-text-muted focus:border-accent-violet/70 focus:bg-white/10 focus:ring-4 focus:ring-accent-violet/10" /></label>}
                <label className="block"><span className="mb-1.5 block text-xs font-medium text-text-secondary">Password</span><div className="flex gap-2"><input type="password" value={password} onChange={(event) => setPassword(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void submitAuth(); }} placeholder="At least 8 characters" autoComplete={authMode === "signup" ? "new-password" : "current-password"} className="min-w-0 flex-1 rounded-xl border border-white/10 bg-white/5 px-4 py-3.5 text-white outline-none transition placeholder:text-text-muted focus:border-accent-violet/70 focus:bg-white/10 focus:ring-4 focus:ring-accent-violet/10" /><button aria-label={authMode === "signup" ? "Create account" : "Sign in"} onClick={() => void submitAuth()} disabled={authBusy} className="w-14 rounded-xl bg-gradient-to-r from-accent-cyan to-accent-violet shadow-lg shadow-accent-violet/20 transition hover:brightness-110 disabled:opacity-50">{authBusy ? <Loader2 className="mx-auto h-5 w-5 animate-spin text-white" /> : <ArrowRight className="mx-auto h-5 w-5 text-white" />}</button></div></label>
              </div>
              {authMode === "signup" && <p className="mt-4 text-xs leading-5 text-text-muted">Your public handle will be generated from your username after registration.</p>}
              {authError && <p role="alert" className="mt-4 rounded-xl border border-accent-pink/20 bg-accent-pink/10 px-3 py-2.5 text-center text-sm text-pink-200">{authError}</p>}
              <div className="my-6 flex items-center gap-3 text-[11px] uppercase tracking-wider text-text-muted"><span className="h-px flex-1 bg-white/10" />or<span className="h-px flex-1 bg-white/10" /></div>
              <a href="/api/user/oauth2/google/login" className="flex w-full items-center justify-center gap-2.5 rounded-xl border border-white/10 bg-white/5 py-3 text-sm font-medium text-text-secondary transition hover:border-white/20 hover:bg-white/10 hover:text-white"><span className="text-lg font-bold">G</span><span>Continue with Google</span></a>
              <p className="mt-6 flex items-center justify-center gap-1.5 text-center text-[11px] text-text-muted"><ShieldCheck className="h-3.5 w-3.5 text-emerald-400" />Private by design. Your account controls your data.</p>
            </motion.div>
          )}

          {step === "done" && (
            <motion.div key="done" initial={{ opacity: 0, scale: 0.96 }} animate={{ opacity: 1, scale: 1 }} className="w-full max-w-4xl">
              <div className="mb-6 flex items-start justify-between gap-4"><div><p className="mb-2 text-xs font-semibold uppercase tracking-[0.25em] text-accent-cyan">Welcome back</p><h1 className="text-3xl font-bold text-gradient">What would you like to do?</h1><p className="mt-2 text-sm text-text-secondary">Start a random conversation, find someone specific, or join a group.</p></div><FriendRequestsPanel /></div>
              {currentUser && <div className="mb-5 flex items-center gap-2 text-xs text-text-secondary"><span className="rounded-full bg-white/10 px-3 py-1.5">{currentUser.name}</span><span className="text-accent-violet">@{currentUser.handle || currentUser.id}</span><span className="text-text-muted">ID: {currentUser.id}</span></div>}
              <div className="grid gap-4 md:grid-cols-3">
                <motion.button whileHover={{ y: -4 }} whileTap={{ scale: 0.98 }} onClick={startMatching} className="glass-strong rounded-3xl p-6 text-left transition hover:border-accent-cyan/40"><Sparkles className="mb-5 h-9 w-9 text-accent-cyan" /><span className="block text-xl font-bold text-gradient">Start matching</span><span className="mt-2 block text-sm leading-6 text-text-secondary">Meet someone who is ready to talk right now.</span></motion.button>
                <motion.button whileHover={{ y: -4 }} whileTap={{ scale: 0.98 }} onClick={() => setStep("search")} className="glass-strong rounded-3xl p-6 text-left transition hover:border-accent-violet/40"><MessageCircle className="mb-5 h-9 w-9 text-accent-violet" /><span className="block text-xl font-bold text-gradient">Find a person</span><span className="mt-2 block text-sm leading-6 text-text-secondary">Search by name, handle, or public ID.</span></motion.button>
                <div className="glass-strong rounded-3xl p-6"><UsersRound className="mb-5 h-9 w-9 text-accent-pink" /><span className="block text-xl font-bold text-gradient">Join a group</span><span className="mt-2 block text-sm leading-6 text-text-secondary">Enter an invite code to join a group chat.</span><div className="mt-5 flex gap-2"><input value={inviteCode} onChange={(event) => setInviteCode(event.target.value.toUpperCase())} placeholder="INVITE CODE" className="min-w-0 flex-1 rounded-xl border border-white/10 bg-white/5 px-3 py-2.5 text-sm uppercase text-white outline-none placeholder:text-text-muted focus:border-accent-pink/60" /><button aria-label="Join group" onClick={() => void joinGroup()} className="rounded-xl bg-accent-pink/20 px-3 text-accent-pink transition hover:bg-accent-pink/30"><Hash className="h-4 w-4" /></button></div>{inviteError && <p role="alert" className="mt-2 text-xs text-accent-pink">{inviteError}</p>}</div>
              </div>
            </motion.div>
          )}

          {step === "search" && <div className="w-full max-w-lg"><UserDiscoveryPanel onOpenChat={openChat} /><button onClick={() => setStep("done")} className="mt-3 w-full text-center text-sm text-text-secondary transition hover:text-white">← Back to options</button></div>}

          {step === "matching" && <motion.div key="matching" initial={{ opacity: 0, scale: 0.9 }} animate={{ opacity: 1, scale: 1 }}><div className="glass-strong rounded-3xl px-12 py-8"><div className="flex flex-col items-center gap-4"><div className="flex gap-2">{[0, 1, 2].map((i) => <motion.div key={i} className="h-3 w-3 rounded-full bg-gradient-to-r from-cyan-400 to-violet-500" animate={{ y: [0, -12, 0], opacity: [0.5, 1, 0.5] }} transition={{ duration: 1.2, repeat: Infinity, delay: i * 0.2 }} />)}</div><span className="text-xl font-semibold text-gradient">Finding someone for you…</span><span className="text-sm text-text-secondary">{matchError || "Please wait while we connect you."}</span><button onClick={cancelMatching} className="mt-2 rounded-xl border border-white/10 px-4 py-2 text-sm text-text-secondary transition hover:bg-white/10 hover:text-white">Cancel</button></div></div></motion.div>}
        </AnimatePresence>
      </div>
    </div>
  );
}
