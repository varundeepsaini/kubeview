"use client";

import React from "react";
import { useCluster } from "./ClusterProvider";
import { useTimeTravel } from "./TimeTravelProvider";

// ClusterScope keys the main content by the active context AND the pinned
// time-travel moment, so switching either remounts the page subtree. Every
// data hook then re-runs from scratch — refetching against the new cluster,
// or resolving the frozen /history/state snapshot — with no manual
// cache-busting in any page.
export default function ClusterScope({
  children,
}: {
  children: React.ReactNode;
}) {
  const { context } = useCluster();
  const { at } = useTimeTravel();

  return (
    <main key={`${context}:${at ?? "live"}`} className="ml-56 min-h-screen p-6">
      {children}
    </main>
  );
}
