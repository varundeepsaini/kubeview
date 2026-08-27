"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { eventSourceUrl } from "./api";

export function usePolling<T>(
  fetcher: () => Promise<T>,
  interval: number = 5000
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const result = await fetcher();
      setData(result);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  }, [fetcher]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, interval);
    return () => clearInterval(id);
  }, [refresh, interval]);

  return { data, error, loading, refresh };
}

const ageTickMs = 30000;

// Watch-driven tables only re-render when a watch event lands, so relative
// times baked in at fetch time would freeze; tick so they stay current.
export function useNow(intervalMs: number = ageTickMs): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

// Client-side equivalent of the backend's getAge (transformers.go), computed
// from createdAt at render time instead of baked into the list response.
export function formatAge(createdAt: string, now: number): string {
  if (!createdAt) return "Unknown";
  const created = new Date(createdAt).getTime();
  if (Number.isNaN(created)) return "Unknown";
  const secs = Math.max(Math.floor((now - created) / 1000), 0);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

interface WatchMessage<T> { type: "added" | "modified" | "deleted" | "error"; resource: string; key: string; object?: T }

function applyWatchMessage<T extends { name: string; namespace?: string }>(current: T[], message: WatchMessage<T>): T[] {
  const keyOf = (item: T) => `${item.namespace ?? ""}/${item.name}`;
  if (message.type === "deleted") return current.filter(item => keyOf(item) !== message.key);
  if (!message.object) return current;
  const index = current.findIndex(item => keyOf(item) === message.key);
  if (index < 0) {
    // Insert in key order to preserve the initial list's namespace/name sort;
    // appending would scramble tables as rows churn.
    const insertAt = current.findIndex(item => keyOf(item) > message.key);
    if (insertAt < 0) return [...current, message.object];
    return [...current.slice(0, insertAt), message.object, ...current.slice(insertAt)];
  }
  const next = [...current]; next[index] = message.object; return next;
}

interface WatchSubscriber {
  resource: string;
  onMessage: (message: WatchMessage<unknown>) => void;
  onRefresh: () => void;
}

interface WatchConnection {
  url: string;
  source: EventSource | null;
  subscribers: Set<WatchSubscriber>;
  reconnectTimer: ReturnType<typeof setTimeout> | null;
  reconnectAttempts: number;
  needsRefresh: boolean;
}

// One EventSource per namespace filter, multiplexing every subscribed
// resource (resources=a,b,c). Browsers cap HTTP/1.1 connections per origin
// at ~6, so per-resource EventSources would starve all other API requests.
const watchConnections = new Map<string, WatchConnection>();
let watchSyncScheduled = false;

const reconnectBaseDelayMs = 1000;
const reconnectMaxDelayMs = 30000;
const listRetryDelayMs = 5000;

function subscribeWatch(namespace: string | undefined, subscriber: WatchSubscriber): () => void {
  const key = namespace ?? "";
  let connection = watchConnections.get(key);
  if (!connection) {
    connection = { url: "", source: null, subscribers: new Set(), reconnectTimer: null, reconnectAttempts: 0, needsRefresh: false };
    watchConnections.set(key, connection);
  }
  connection.subscribers.add(subscriber);
  scheduleWatchSync();
  return () => {
    connection.subscribers.delete(subscriber);
    scheduleWatchSync();
  };
}

// Subscription changes from one React commit coalesce into a single
// (re)connect instead of reopening the stream once per hook.
function scheduleWatchSync() {
  if (watchSyncScheduled) return;
  watchSyncScheduled = true;
  queueMicrotask(() => {
    watchSyncScheduled = false;
    for (const [key, connection] of watchConnections) syncWatchConnection(key, connection);
  });
}

function syncWatchConnection(key: string, connection: WatchConnection) {
  if (connection.subscribers.size === 0) {
    closeWatchConnection(connection);
    watchConnections.delete(key);
    return;
  }
  const resources = [...new Set([...connection.subscribers].map(s => s.resource))].sort();
  const params = new URLSearchParams({ resources: resources.join(",") });
  if (key) params.set("namespace", key);
  const url = eventSourceUrl(`/watch?${params}`);
  if (connection.source && connection.url === url) return;
  // Replacing a live stream can drop events until the new one is open, so
  // surviving subscribers must re-list once it is.
  if (connection.source) connection.needsRefresh = true;
  connection.url = url;
  openWatchConnection(connection);
}

function closeWatchConnection(connection: WatchConnection) {
  if (connection.reconnectTimer) clearTimeout(connection.reconnectTimer);
  connection.reconnectTimer = null;
  connection.source?.close();
  connection.source = null;
}

function openWatchConnection(connection: WatchConnection) {
  closeWatchConnection(connection);
  const source = new EventSource(connection.url);
  connection.source = source;
  source.onopen = () => {
    connection.reconnectAttempts = 0;
    if (!connection.needsRefresh) return;
    connection.needsRefresh = false;
    for (const subscriber of connection.subscribers) subscriber.onRefresh();
  };
  source.addEventListener("resource", (raw) => {
    const message = JSON.parse((raw as MessageEvent).data) as WatchMessage<unknown>;
    for (const subscriber of connection.subscribers) {
      if (subscriber.resource === message.resource) subscriber.onMessage(message);
    }
  });
  source.onerror = () => {
    // Events may have been dropped, but refreshing now would race the
    // reconnect: a delete landing before the new watch is live would never be
    // seen. Flag it and let onopen re-list once the watch is established.
    connection.needsRefresh = true;
    if (source.readyState !== EventSource.CLOSED || connection.source !== source) return;
    // A non-200 response closes the EventSource for good (browsers only
    // auto-reconnect established streams), so recreate it with backoff.
    const delay = Math.min(reconnectBaseDelayMs * 2 ** connection.reconnectAttempts, reconnectMaxDelayMs);
    connection.reconnectAttempts += 1;
    // A watch that fails persistently (e.g. RBAC grants list but not watch)
    // would otherwise freeze tables on their initial snapshot with no signal:
    // once past the transient-blip attempts, re-list on each failed attempt so
    // data still converges. onopen re-lists again if the watch ever recovers.
    if (connection.reconnectAttempts > 2) {
      for (const subscriber of connection.subscribers) subscriber.onRefresh();
    }
    connection.reconnectTimer = setTimeout(() => openWatchConnection(connection), delay);
  };
}

export function useWatchList<T extends { name: string; namespace?: string }>(fetcher: () => Promise<T[]>, resource: string, namespace?: string) {
  const [data, setData] = useState<T[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  // Watch events that arrive while a list fetch is in flight; replayed on top
  // of the snapshot so a stale list cannot overwrite newer reconciled state.
  const pending = useRef<WatchMessage<T>[] | null>(null);
  // Only the latest of overlapping refreshes may apply its snapshot and
  // release the pending buffer, or a stale list could resurrect deleted rows.
  const refreshToken = useRef(0);
  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const refresh = useCallback(async () => {
    if (retryTimer.current) { clearTimeout(retryTimer.current); retryTimer.current = null; }
    const token = ++refreshToken.current;
    pending.current = [];
    try {
      const snapshot = await fetcher();
      if (token !== refreshToken.current) return;
      const buffered = pending.current ?? [];
      setData(buffered.reduce(applyWatchMessage, snapshot));
      setError(null);
    }
    catch (err) {
      if (token !== refreshToken.current) return;
      // The snapshot failed, but events buffered during the fetch are still
      // real; apply them to the existing data so e.g. deletes are not lost.
      const buffered = pending.current ?? [];
      if (buffered.length > 0) setData(current => current ? buffered.reduce(applyWatchMessage, current) : current);
      setError(err instanceof Error ? err.message : "Unknown error");
      // A healthy watch never re-triggers refresh, so a transient list
      // failure would otherwise leave the error screen up forever; retry.
      retryTimer.current = setTimeout(() => { void refresh(); }, listRetryDelayMs);
    }
    finally {
      if (token === refreshToken.current) { pending.current = null; setLoading(false); }
    }
  }, [fetcher]);
  useEffect(() => {
    refresh();
    const unsubscribe = subscribeWatch(namespace, {
      resource,
      onMessage: (raw) => {
        const message = raw as WatchMessage<T>;
        if (pending.current) { pending.current.push(message); return; }
        setData(current => {
          // Deleted events carry the object's last state; don't materialize it.
          if (!current) return message.type !== "deleted" && message.object ? [message.object] : current;
          return applyWatchMessage(current, message);
        });
      },
      onRefresh: refresh,
    });
    return () => {
      unsubscribe();
      if (retryTimer.current) { clearTimeout(retryTimer.current); retryTimer.current = null; }
      // Invalidate any in-flight refresh so its failure path cannot schedule
      // a new retry after cleanup, orphaning a polling loop.
      refreshToken.current += 1;
    };
  }, [namespace, refresh, resource]);
  return { data, error, loading, refresh };
}
