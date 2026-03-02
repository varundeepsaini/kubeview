"use client";

import { useCallback } from "react";
import { api, NodeInfo } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import StatusBadge from "@/components/StatusBadge";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";

export default function NodesPage() {
  const fetcher = useCallback(() => api.getNodes(), []);
  const { data, error, loading, refresh } = usePolling<NodeInfo[]>(fetcher);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-bold">Nodes</h1>
        <p className="text-muted text-sm mt-1">{data?.length || 0} nodes</p>
      </div>

      <div className="space-y-4">
        {data?.map((node) => (
          <div key={node.name} className="bg-card border border-border rounded-xl p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-3">
                <h3 className="text-lg font-semibold">{node.name}</h3>
                <StatusBadge status={node.status} size="md" />
                {node.roles.map((role) => (
                  <span key={role} className="px-2 py-0.5 bg-accent-purple/15 text-accent-purple rounded text-xs font-medium">
                    {role}
                  </span>
                ))}
              </div>
              <span className="text-sm text-muted">{node.age}</span>
            </div>

            <div className="grid grid-cols-4 gap-4">
              <div className="bg-background border border-border rounded-lg p-4">
                <p className="text-xs text-muted mb-1">Version</p>
                <p className="font-mono text-sm">{node.version}</p>
              </div>
              <div className="bg-background border border-border rounded-lg p-4">
                <p className="text-xs text-muted mb-1">OS / Arch</p>
                <p className="text-sm truncate">{node.os}</p>
                <p className="text-xs text-muted">{node.arch}</p>
              </div>
              <div className="bg-background border border-border rounded-lg p-4">
                <p className="text-xs text-muted mb-1">Resources</p>
                <p className="text-sm">CPU: {node.cpu} | Memory: {node.memory}</p>
              </div>
              <div className="bg-background border border-border rounded-lg p-4">
                <p className="text-xs text-muted mb-1">Container Runtime</p>
                <p className="text-sm truncate">{node.containerRuntime}</p>
              </div>
            </div>

            {node.addresses.length > 0 && (
              <div className="mt-4 flex gap-4">
                {node.addresses.map((addr, i) => (
                  <div key={`${addr.type}-${i}`} className="text-sm">
                    <span className="text-muted">{addr.type}: </span>
                    <span className="font-mono text-xs">{addr.address}</span>
                  </div>
                ))}
              </div>
            )}

            <div className="mt-4">
              <p className="text-xs text-muted mb-2">Conditions</p>
              <div className="flex flex-wrap gap-2">
                {node.conditions.map((c) => (
                  <span
                    key={c.type}
                    className={`px-2 py-1 rounded text-xs ${
                      c.status === "True" && c.type === "Ready"
                        ? "bg-emerald-500/15 text-emerald-400"
                        : c.status === "False"
                        ? "bg-emerald-500/15 text-emerald-400"
                        : "bg-yellow-500/15 text-yellow-400"
                    }`}
                  >
                    {c.type}: {c.status}
                  </span>
                ))}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
