"use client";

import { useEffect, useState } from "react";
import { api, ContextInfo } from "@/lib/api";
import { useCluster } from "./ClusterProvider";

// Delay before re-fetching the context list after a failed attempt, matching
// the resource pages' poll interval.
const RETRY_DELAY_MS = 5000;

// ContextSwitcher lists the kubeconfig contexts and switches the active one.
// The context list is read from the backend's kubeconfig (not a live cluster),
// so it loads even when the current context is unreachable — giving the user a
// way back. Contexts don't change without a backend restart, so this fetches
// once on mount rather than polling — but a failure (e.g. the backend still
// starting) is retried, since giving up would permanently hide the switcher
// and keep the stale-context fallback below from ever running.
export default function ContextSwitcher() {
  const { context, setContext } = useCluster();
  const [contexts, setContexts] = useState<ContextInfo[]>([]);

  useEffect(() => {
    let active = true;
    let retry: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      api
        .getContexts()
        .then((list) => active && setContexts(list))
        .catch(() => {
          if (active) {
            retry = setTimeout(load, RETRY_DELAY_MS);
          }
        });
    };
    load();
    return () => {
      active = false;
      clearTimeout(retry);
    };
  }, []);

  // A persisted context the backend no longer knows (renamed, deleted, or a
  // different backend such as in-cluster) makes every resource request 400,
  // and with a single context there is no select to recover from — so fall
  // back to the backend default.
  useEffect(() => {
    if (
      context &&
      contexts.length > 0 &&
      !contexts.some((c) => c.name === context)
    ) {
      setContext("");
    }
  }, [context, contexts, setContext]);

  if (contexts.length === 0) {
    return null;
  }

  // An empty selection means "backend default" — reflect that in the control by
  // falling back to the context the backend reports as current.
  const current = contexts.find((c) => c.current)?.name ?? "";
  const value = context || current;

  if (contexts.length === 1) {
    return (
      <div className="mt-3">
        <p className="text-[10px] uppercase tracking-wide text-muted mb-1">
          Context
        </p>
        <p className="text-sm truncate" title={contexts[0].name}>
          {contexts[0].name}
        </p>
      </div>
    );
  }

  return (
    <div className="mt-3">
      <label
        htmlFor="context-switcher"
        className="text-[10px] uppercase tracking-wide text-muted mb-1 block"
      >
        Context
      </label>
      <select
        id="context-switcher"
        value={value}
        onChange={(e) => setContext(e.target.value)}
        className="w-full bg-card border border-border rounded-lg px-2 py-2 text-sm text-foreground focus:outline-none focus:border-accent/50"
      >
        {contexts.map((c) => (
          <option key={c.name} value={c.name}>
            {c.name}
            {c.current ? " (current)" : ""}
          </option>
        ))}
      </select>
    </div>
  );
}
