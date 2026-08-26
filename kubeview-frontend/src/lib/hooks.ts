"use client";

import { useState, useEffect, useCallback } from "react";
import { eventSourceUrl } from "./api";

export function usePolling<T>(
  fetcher: () => Promise<T>,
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
  }, [refresh]);

  return { data, error, loading, refresh };
}

interface WatchMessage<T> { type: "added" | "modified" | "deleted" | "error"; key: string; object?: T }

export function useWatchList<T extends { name: string; namespace?: string }>(fetcher: () => Promise<T[]>, resource: string, namespace?: string) {
  const [data, setData] = useState<T[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const refresh = useCallback(async () => {
    try { setData(await fetcher()); setError(null); }
    catch (err) { setError(err instanceof Error ? err.message : "Unknown error"); }
    finally { setLoading(false); }
  }, [fetcher]);
  useEffect(() => {
    refresh();
    const params = new URLSearchParams({ resources: resource });
    if (namespace) params.set("namespace", namespace);
    const source = new EventSource(eventSourceUrl(`/watch?${params}`));
    source.addEventListener("resource", (raw) => {
      const message = JSON.parse((raw as MessageEvent).data) as WatchMessage<T>;
      setData(current => {
        if (!current) return message.object ? [message.object] : current;
        const keyOf = (item: T) => `${item.namespace ?? ""}/${item.name}`;
        if (message.type === "deleted") return current.filter(item => keyOf(item) !== message.key);
        if (!message.object) return current;
        const index = current.findIndex(item => keyOf(item) === message.key);
        if (index < 0) return [...current, message.object];
        const next = [...current]; next[index] = message.object; return next;
      });
    });
    source.onerror = () => { refresh(); };
    return () => source.close();
  }, [namespace, refresh, resource]);
  return { data, error, loading, refresh };
}
