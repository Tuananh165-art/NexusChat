"use client";

import { useRef, useCallback, useEffect } from "react";
import type { Message } from "@/lib/constants";

type WebSocketStatus = "connecting" | "connected" | "disconnected";

interface UseWebSocketOptions {
  onMessage: (msg: Message) => void;
  onStatusChange: (status: WebSocketStatus) => void;
}

type WebSocketURL = string | (() => Promise<string>);

export function useWebSocket({ onMessage, onStatusChange }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const shouldReconnect = useRef(true);
  const reconnectAttempts = useRef(0);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const onMessageRef = useRef(onMessage);
  const onStatusChangeRef = useRef(onStatusChange);
  const urlFactoryRef = useRef<WebSocketURL | null>(null);

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

  const connect = useCallback((urlFactory: WebSocketURL) => {
    // Prevent multiple connections.
    if (wsRef.current?.readyState === WebSocket.OPEN ||
        wsRef.current?.readyState === WebSocket.CONNECTING) {
      return;
    }

    urlFactoryRef.current = urlFactory;
    reconnectAttempts.current = 0;
    shouldReconnect.current = true;

    const scheduleReconnect = (doConnect: () => void) => {
      if (!shouldReconnect.current) return;
      reconnectAttempts.current++;
      const exponential = Math.min(1000 * Math.pow(2, reconnectAttempts.current - 1), 30000);
      const delay = Math.round(exponential * (0.8 + Math.random() * 0.4));
      reconnectTimeoutRef.current = setTimeout(doConnect, delay);
    };

    const doConnect = async () => {
      cleanup();
      if (!shouldReconnect.current) return;
      onStatusChangeRef.current("connecting");

      let url: string;
      try {
        const currentFactory = urlFactoryRef.current;
        if (!currentFactory) return;
        url = typeof currentFactory === "function" ? await currentFactory() : currentFactory;
        if (!shouldReconnect.current) return;
      } catch (err) {
        console.error("WebSocket ticket request failed:", err);
        onStatusChangeRef.current("disconnected");
        scheduleReconnect(() => void doConnect());
        return;
      }

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
            // Ignore malformed frames from the server.
          }
        };

        ws.onclose = () => {
          // An old socket can close after a room switch. It must not mark or
          // reconnect over the newer socket that replaced it.
          if (wsRef.current !== ws) return;
          onStatusChangeRef.current("disconnected");
          wsRef.current = null;
          scheduleReconnect(() => void doConnect());
        };

        ws.onerror = () => {
          // The close event carries the reconnect path.
        };
      } catch (err) {
        console.error("WebSocket connection error:", err);
        onStatusChangeRef.current("disconnected");
        scheduleReconnect(() => void doConnect());
      }
    };

    void doConnect();
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
