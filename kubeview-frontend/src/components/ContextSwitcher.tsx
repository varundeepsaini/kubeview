"use client";

import { useEffect, useState } from "react";
import { api, ContextInfo } from "@/lib/api";
import { useCluster } from "./ClusterProvider";

// ContextSwitcher lists the kubeconfig contexts and switches the active one.
// The context list is read from the backend's kubeconfig (not a live cluster),
// so it loads even when the current context is unreachable — giving the user a
// way back. Contexts don't change without a backend restart, so this fetches
// once on mount rather than polling.
export default function ContextSwitcher() {
  const { context, setContext } = useCluster();
  const [contexts, setContexts] = useState<ContextInfo[]>([]);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let active = true;
    api
      .getContexts()
      .then((list) => active && setContexts(list))
      .catch(() => active && setFailed(true));
    return () => {
      active = false;
    };
  }, []);

  if (failed || contexts.length === 0) {
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
