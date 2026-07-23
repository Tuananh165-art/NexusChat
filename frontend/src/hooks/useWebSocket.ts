"use client";

import { useRef, useCallback, useEffect } from "react";
import type { Message } from "@/lib/constants";

type WebSocketStatus = "connecting" | "connected" | "disconnected";

interface UseWebSocketOptions {
  onMessage: (msg: Message) => void;
  onStatusChange: (status: WebSocketStatus) => void;
}

export function useWebSocket({ onMessage, onStatusChange }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const shouldReconnect = useRef(true);
  const reconnectAttempts = useRef(0);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const onMessageRef = useRef(onMessage);
  const onStatusChangeRef = useRef(onStatusChange);
  const urlRef = useRef<string>("");

  useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  useEffect(() => {
    onStatusChangeRef.current = onStatusChange;
  }, [onStatusChange]);

  const cleanup = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = undefined;
    }
    if (wsRef.current) {
      wsRef.current.onopen = null;
      wsRef.current.onmessage = null;
      wsRef.current.onclose = null;
      wsRef.current.onerror = null;
      if (wsRef.current.readyState === WebSocket.OPEN || 
          wsRef.current.readyState === WebSocket.CONNECTING) {
        wsRef.current.close();
      }
      wsRef.current = null;
    }
  }, []);

  const connect = useCallback((url: string) => {
    // Prevent multiple connections
    if (wsRef.current?.readyState === WebSocket.OPEN || 
        wsRef.current?.readyState === WebSocket.CONNECTING) {
      return;
    }

    urlRef.current = url;
    reconnectAttempts.current = 0;
    shouldReconnect.current = true;

    const doConnect = () => {
      cleanup();
      
      if (!shouldReconnect.current) return;

      onStatusChangeRef.current("connecting");

      try {
        const ws = new WebSocket(url);
        wsRef.current = ws;

        ws.onopen = () => {
          reconnectAttempts.current = 0;
          onStatusChangeRef.current("connected");
        };

        ws.onmessage = (e) => {
          try {
            const msg = JSON.parse(e.data) as Message;
            onMessageRef.current(msg);
          } catch {
            // ignore parse errors
          }
        };

        ws.onclose = () => {
          onStatusChangeRef.current("disconnected");
          wsRef.current = null;
          
          if (shouldReconnect.current && reconnectAttempts.current < 10) {
            reconnectAttempts.current++;
            const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current - 1), 30000);
            reconnectTimeoutRef.current = setTimeout(doConnect, delay);
          }
        };

        ws.onerror = () => {
          // Error will trigger onclose
        };
      } catch (err) {
        console.error("WebSocket connection error:", err);
        onStatusChangeRef.current("disconnected");
      }
    };

    doConnect();
  }, [cleanup]);

  const send = useCallback((msg: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    } else {
      throw new Error("WebSocket is not open");
    }
  }, []);

  const disconnect = useCallback(() => {
    shouldReconnect.current = false;
    cleanup();
  }, [cleanup]);

  useEffect(() => {
    return () => {
      shouldReconnect.current = false;
      cleanup();
    };
  }, [cleanup]);

  return { connect, send, disconnect };
}
