"use client";

import React, { createContext, useContext, useState } from "react";
import { setApiContext } from "@/lib/api";

const STORAGE_KEY = "kubeview.context";

interface ClusterContextValue {
  // context is the selected kubeconfig context, or "" to use the backend's
  // default (current) context.
  context: string;
  setContext: (name: string) => void;
}

const ClusterContext = createContext<ClusterContextValue | null>(null);

// readSaved returns the persisted context. It runs during the useState
// initializer (both on the server, where window is absent, and on the client),
// syncing the api module before any child page's fetch effect fires — so the
// first request already carries the right context.
function readSaved(): string {
  if (typeof window === "undefined") {
    return "";
  }
  return localStorage.getItem(STORAGE_KEY) ?? "";
}

export function ClusterProvider({ children }: { children: React.ReactNode }) {
  const [context, setContextState] = useState<string>(() => {
    const saved = readSaved();
    setApiContext(saved);
    return saved;
  });

  const setContext = (name: string) => {
    setApiContext(name);
    if (name) {
      localStorage.setItem(STORAGE_KEY, name);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
    setContextState(name);
  };

  return (
    <ClusterContext.Provider value={{ context, setContext }}>
      {children}
    </ClusterContext.Provider>
  );
}

export function useCluster(): ClusterContextValue {
  const ctx = useContext(ClusterContext);
  if (!ctx) {
    throw new Error("useCluster must be used within a ClusterProvider");
  }
  return ctx;
}
