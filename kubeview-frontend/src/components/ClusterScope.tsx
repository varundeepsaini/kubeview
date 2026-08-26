"use client";

import React from "react";
import { useCluster } from "./ClusterProvider";

// ClusterScope keys the main content by the active context so switching
// contexts remounts the page subtree. Every usePolling hook then re-runs from
// scratch, clearing stale data and refetching against the new cluster with no
// manual cache-busting.
export default function ClusterScope({
  children,
}: {
  children: React.ReactNode;
}) {
  const { context } = useCluster();

  return (
    <main key={context} className="ml-56 min-h-screen p-6">
      {children}
    </main>
  );
}
