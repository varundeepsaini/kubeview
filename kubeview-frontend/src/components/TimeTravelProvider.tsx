"use client";

import React, { createContext, useCallback, useContext, useState } from "react";
import { setApiAt } from "@/lib/api";
import { useCluster } from "./ClusterProvider";

interface TimeTravelValue {
  // at is the pinned past moment (unix ms), or null when viewing live.
  at: number | null;
  setAt: (at: number | null) => void;
}

// pin remembers the context a moment was pinned in: a moment from one
// cluster is meaningless — and misleading — under another.
interface TimeTravelPin {
  context: string;
  at: number | null;
}

const TimeTravelContext = createContext<TimeTravelValue | null>(null);

// TimeTravelProvider mirrors the scrubber's pinned moment into the api
// module (like ClusterProvider does for the context) before updating React
// state, so the remounted pages' very first fetches already resolve from
// history. Deliberately not persisted: a reload always returns to live.
export function TimeTravelProvider({ children }: { children: React.ReactNode }) {
  const { context } = useCluster();
  const [pin, setPin] = useState<TimeTravelPin>({ context, at: null });

  const setAt = useCallback(
    (next: number | null) => {
      setApiAt(next);
      setPin({ context, at: next });
    },
    [context]
  );

  // Reset during render (not in an effect) on context switch, so no child
  // ever renders — or fetches — the old pinned moment under the new context:
  // switching clusters always returns the dashboard to live view.
  if (pin.context !== context) {
    setApiAt(null);
    setPin({ context, at: null });
  }

  const at = pin.context === context ? pin.at : null;

  return (
    <TimeTravelContext.Provider value={{ at, setAt }}>
      {children}
    </TimeTravelContext.Provider>
  );
}

export function useTimeTravel(): TimeTravelValue {
  const ctx = useContext(TimeTravelContext);
  if (!ctx) {
    throw new Error("useTimeTravel must be used within a TimeTravelProvider");
  }
  return ctx;
}
