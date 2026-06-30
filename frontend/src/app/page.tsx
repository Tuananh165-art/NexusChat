"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import { ArrowRight, Sparkles, Loader2 } from "lucide-react";
import { ACCESS_TOKEN_KEY } from "@/lib/constants";
import { fetchMe, createUser } from "@/lib/api";
import AnimatedBackground from "@/components/AnimatedBackground";

export default function HomePage() {
  const router = useRouter();
  const [step, setStep] = useState<"input" | "done" | "matching">("input");
  const [name, setName] = useState("");
  const [error, setError] = useState(false);
  const [matchError, setMatchError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const inputRef = useRef<HTMLInputElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const cardRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    (async () => {
      try {
        const me = await fetchMe();
        localStorage.setItem("rc:userid", me.id);
        localStorage.setItem("rc:username", me.name);
        localStorage.setItem("rc:userpicture", me.picture || "");
        const token = localStorage.getItem(ACCESS_TOKEN_KEY);
        if (token) {
          router.push("/chat");
        } else {
          setStep("done");
        }
      } catch {
        localStorage.removeItem(ACCESS_TOKEN_KEY);
        localStorage.removeItem("rc:userid");
        localStorage.removeItem("rc:username");
        localStorage.removeItem("rc:userpicture");
        setStep("input");
      } finally {
        setIsLoading(false);
      }
    })();
  }, [router]);

  useEffect(() => {
    if (step === "input" && !isLoading && inputRef.current) {
      inputRef.current.focus();
    }
  }, [step, isLoading]);

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!cardRef.current) return;
    const rect = cardRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    const centerX = rect.width / 2;
    const centerY = rect.height / 2;
    const rotateX = (y - centerY) / 20;
    const rotateY = (centerX - x) / 20;
    cardRef.current.style.transform = `perspective(1000px) rotateX(${rotateX}deg) rotateY(${rotateY}deg) scale3d(1.02, 1.02, 1.02)`;
  };

  const handleMouseLeave = () => {
    if (!cardRef.current) return;
    cardRef.current.style.transform = "perspective(1000px) rotateX(0deg) rotateY(0deg) scale3d(1, 1, 1)";
  };

  const validate = async () => {
    if (!name.trim() || name.length > 15) {
      setError(true);
      setTimeout(() => setError(false), 700);
      return;
    }
    try {
      const res = await fetch("/api/user", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (res.status !== 201) throw new Error(res.statusText);
      const user = await res.json();
      localStorage.setItem("rc:userid", user.id);
      localStorage.setItem("rc:username", user.name);
      localStorage.setItem("rc:userpicture", user.picture || "");
      setTimeout(() => setStep("done"), 300);
    } catch {
      setError(true);
      setTimeout(() => setError(false), 700);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") validate();
  };

  const startMatching = () => {
    setMatchError("");
    setStep("matching");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const matchUrl = `${protocol}//${window.location.host}/api/match`;
    const ws = new WebSocket(matchUrl);
    wsRef.current = ws;

    ws.addEventListener("message", (e) => {
      const result = JSON.parse(e.data);
      if (result.channel_id !== "" && result.access_token !== "") {
        localStorage.setItem(ACCESS_TOKEN_KEY, result.access_token);
        wsRef.current = null;
        ws.close();
        router.push("/chat");
      }
    });

    ws.addEventListener("error", () => {
      setMatchError("Unable to connect to matching service");
    });

    ws.addEventListener("close", () => {
      if (wsRef.current === ws) {
        setMatchError("Matching connection closed");
      }
    });
  };

  const cancelMatching = () => {
    if (wsRef.current) {
      const ws = wsRef.current;
      wsRef.current = null;
      ws.close();
    }
    setMatchError("");
    setStep("done");
  };

  useEffect(() => {
    return () => {
      if (wsRef.current) {
        const ws = wsRef.current;
        wsRef.current = null;
        ws.close();
      }
    };
  }, []);

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center relative">
        <AnimatedBackground />
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          className="relative z-10"
        >
          <Loader2 className="w-8 h-8 text-accent-violet animate-spin" />
        </motion.div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center relative overflow-hidden">
      <AnimatedBackground />

      <div className="relative z-10 w-full flex items-center justify-center px-4">
        <AnimatePresence mode="wait">
          {step === "input" && (
            <motion.div
              key="input"
              initial={{ opacity: 0, y: 40, scale: 0.95 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -40, scale: 0.95 }}
              transition={{ type: "spring", stiffness: 200, damping: 25 }}
              className="perspective-1000 w-full max-w-md"
            >
              <div
                ref={cardRef}
                onMouseMove={handleMouseMove}
                onMouseLeave={handleMouseLeave}
                className="preserve-3d glass-strong rounded-3xl p-8 transition-transform duration-200 ease-out"
              >
                <motion.div
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.2 }}
                  className="text-center mb-8"
                >
                  <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-gradient-to-br from-accent-cyan/20 to-accent-violet/20 border border-white/10 mb-4">
                    <Sparkles className="w-8 h-8 text-accent-cyan" />
                  </div>
                  <h1 className="text-3xl font-bold text-gradient mb-2">
                    NexusChat
                  </h1>
                  <p className="text-text-secondary text-sm">
                    Connect with strangers instantly
                  </p>
                </motion.div>

                <motion.div
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.3 }}
                  className={`relative ${error ? "animate-shake" : ""}`}
                >
                  <div className="relative group">
                    <div className="absolute inset-0 bg-gradient-to-r from-accent-cyan/20 via-accent-violet/20 to-accent-pink/20 rounded-xl blur-lg opacity-0 group-focus-within:opacity-100 transition-opacity duration-500" />
                    <div className="relative flex items-center gap-3 glass rounded-xl px-4 py-3 border border-white/10 group-focus-within:border-accent-violet/50 transition-all duration-300">
                      <input
                        ref={inputRef}
                        type="text"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="Enter your name..."
                        maxLength={15}
                        className="flex-1 bg-transparent border-none outline-none text-text-primary text-lg font-medium placeholder:text-text-muted"
                        autoFocus
                      />
                      <motion.button
                        whileHover={{ scale: 1.1 }}
                        whileTap={{ scale: 0.9 }}
                        onClick={validate}
                        className="w-10 h-10 rounded-xl bg-gradient-to-r from-accent-cyan to-accent-violet flex items-center justify-center cursor-pointer border-none shadow-lg shadow-accent-violet/20 hover:shadow-accent-violet/40 transition-shadow"
                      >
                        <ArrowRight className="w-5 h-5 text-white" />
                      </motion.button>
                    </div>
                  </div>
                  {error && (
                    <motion.p
                      initial={{ opacity: 0, y: -10 }}
                      animate={{ opacity: 1, y: 0 }}
                      className="text-accent-pink text-sm mt-2 text-center"
                    >
                      Name must be 1-15 characters
                    </motion.p>
                  )}
                </motion.div>

                <motion.div
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  transition={{ delay: 0.5 }}
                  className="mt-6 text-center"
                >
                  <a
                    href="/api/user/oauth2/google/login"
                    className="inline-flex items-center gap-2 text-text-secondary hover:text-text-primary transition-colors text-sm group"
                  >
                    <svg className="w-4 h-4" viewBox="0 0 24 24">
                      <path fill="currentColor" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/>
                      <path fill="currentColor" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
                      <path fill="currentColor" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
                      <path fill="currentColor" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
                    </svg>
                    <span className="group-hover:underline">Continue with Google</span>
                  </a>
                </motion.div>
              </div>
            </motion.div>
          )}

          {step === "done" && (
            <motion.div
              key="done"
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.8 }}
              transition={{ type: "spring", stiffness: 200, damping: 20 }}
            >
              <motion.button
                whileHover={{ scale: 1.05, boxShadow: "0 0 40px rgba(139, 92, 246, 0.4)" }}
                whileTap={{ scale: 0.95 }}
                onClick={startMatching}
                className="glass-strong rounded-3xl px-12 py-8 cursor-pointer border-none group relative overflow-hidden"
              >
                <div className="absolute inset-0 bg-gradient-to-r from-accent-cyan/10 via-accent-violet/10 to-accent-pink/10 opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
                <div className="relative flex flex-col items-center gap-3">
                  <motion.div
                    animate={{ rotate: [0, 10, -10, 0] }}
                    transition={{ repeat: Infinity, duration: 2, ease: "easeInOut" }}
                  >
                    <Sparkles className="w-10 h-10 text-accent-cyan" />
                  </motion.div>
                  <span className="text-2xl font-bold text-gradient">
                    Start Matching
                  </span>
                  <span className="text-text-secondary text-sm">
                    Find someone to chat with
                  </span>
                </div>
              </motion.button>
            </motion.div>
          )}

          {step === "matching" && (
            <motion.div
              key="matching"
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ type: "spring", stiffness: 200, damping: 20 }}
            >
              <div className="glass-strong rounded-3xl px-12 py-8 relative overflow-hidden">
                <div className="absolute inset-0 bg-gradient-animated opacity-10" />
                <div className="relative flex flex-col items-center gap-4">
                  <div className="flex gap-2">
                    {[0, 1, 2].map((i) => (
                      <motion.div
                        key={i}
                        className="w-3 h-3 rounded-full"
                        style={{ background: `linear-gradient(135deg, #06b6d4, #8b5cf6)` }}
                        animate={{
                          y: [0, -12, 0],
                          scale: [1, 1.2, 1],
                          opacity: [0.5, 1, 0.5],
                        }}
                        transition={{
                          duration: 1.2,
                          repeat: Infinity,
                          delay: i * 0.2,
                          ease: "easeInOut",
                        }}
                      />
                    ))}
                  </div>
                  <span className="text-xl font-semibold text-gradient">
                    Finding someone for you...
                  </span>
                  <span className="text-text-secondary text-sm">
                    {matchError || "Please wait while we connect you"}
                  </span>
                  <button
                    onClick={cancelMatching}
                    className="mt-2 px-4 py-2 rounded-xl glass hover:bg-white/10 transition-all cursor-pointer text-sm text-text-secondary border border-white/10"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  );
}
