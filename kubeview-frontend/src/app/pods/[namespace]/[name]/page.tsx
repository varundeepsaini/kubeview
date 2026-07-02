"use client";

import { useCallback, useState, use } from "react";
import { api, Pod } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import StatusBadge from "@/components/StatusBadge";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";
import Link from "next/link";

export default function PodDetailPage({ params }: { params: Promise<{ namespace: string; name: string }> }) {
  const { namespace, name } = use(params);
  const [activeTab, setActiveTab] = useState<"overview" | "logs">("overview");
  const [selectedContainer, setSelectedContainer] = useState<string>("");
  const [logs, setLogs] = useState<string>("");
  const [logsLoading, setLogsLoading] = useState(false);

  const fetcher = useCallback(() => api.getPod(namespace, name), [namespace, name]);
  const { data: pod, error, loading, refresh } = usePolling<Pod>(fetcher);

  const fetchLogs = async (container?: string) => {
    setLogsLoading(true);
    try {
      const res = await api.getPodLogs(namespace, name, container || undefined);
      setLogs(res.logs);
    } catch (err) {
      setLogs(err instanceof Error ? `Error: ${err.message}` : "Failed to fetch logs");
    }
    setLogsLoading(false);
  };

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;
  if (!pod) return <ErrorMessage message="Pod not found" />;

  // Multi-container pods reject log requests without an explicit container,
  // so always target a concrete container, defaulting to the first one.
  const activeContainer = selectedContainer || pod.containers[0]?.name || "";

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center gap-2 text-sm text-muted mb-2">
          <Link href="/pods" className="hover:text-foreground">Pods</Link>
          <span>/</span>
          <span>{namespace}</span>
          <span>/</span>
          <span className="text-foreground">{name}</span>
        </div>
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold">{name}</h1>
          <StatusBadge status={pod.status} size="md" />
        </div>
        <p className="text-muted text-sm mt-1">Namespace: {namespace} | Node: {pod.node} | IP: {pod.ip}</p>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-6 border-b border-border">
        {(["overview", "logs"] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => {
              setActiveTab(tab);
              if (tab === "logs" && !logs) fetchLogs(activeContainer);
            }}
            className={`px-4 py-2.5 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === tab
                ? "border-accent text-accent"
                : "border-transparent text-muted hover:text-foreground"
            }`}
          >
            {tab.charAt(0).toUpperCase() + tab.slice(1)}
          </button>
        ))}
      </div>

      {activeTab === "overview" ? (
        <div className="space-y-6">
          {/* Containers */}
          <div className="bg-card border border-border rounded-xl p-5">
            <h2 className="text-lg font-semibold mb-4">Containers ({pod.containers.length})</h2>
            <div className="space-y-3">
              {pod.containers.map((c) => (
                <div key={c.name} className="bg-background border border-border rounded-lg p-4">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-3">
                      <span className="font-medium">{c.name}</span>
                      <StatusBadge status={c.state} />
                    </div>
                    <span className="text-xs text-muted">Restarts: {c.restartCount}</span>
                  </div>
                  <div className="text-sm text-muted">
                    <span className="font-mono text-xs">{c.image}</span>
                  </div>
                  {c.ports.length > 0 && (
                    <div className="mt-2 flex gap-2">
                      {c.ports.map((p) => (
                        <span key={p} className="px-2 py-0.5 bg-white/5 rounded text-xs font-mono">{p}</span>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>

          {/* Conditions */}
          <div className="bg-card border border-border rounded-xl p-5">
            <h2 className="text-lg font-semibold mb-4">Conditions</h2>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted text-xs border-b border-border">
                    <th className="text-left pb-3 font-medium">Type</th>
                    <th className="text-left pb-3 font-medium">Status</th>
                    <th className="text-left pb-3 font-medium">Reason</th>
                    <th className="text-left pb-3 font-medium">Last Transition</th>
                  </tr>
                </thead>
                <tbody>
                  {pod.conditions.map((c) => (
                    <tr key={c.type} className="border-b border-border/50">
                      <td className="py-2.5">{c.type}</td>
                      <td className="py-2.5">
                        <span className={`text-xs font-medium ${c.status === "True" ? "text-emerald-400" : "text-red-400"}`}>
                          {c.status}
                        </span>
                      </td>
                      <td className="py-2.5 text-muted text-xs">{c.reason || "-"}</td>
                      <td className="py-2.5 text-muted text-xs">
                        {c.lastTransition ? new Date(c.lastTransition).toLocaleString() : "-"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Volumes */}
          {pod.volumes.length > 0 && (
            <div className="bg-card border border-border rounded-xl p-5">
              <h2 className="text-lg font-semibold mb-4">Volumes ({pod.volumes.length})</h2>
              <div className="grid grid-cols-3 gap-3">
                {pod.volumes.map((v) => (
                  <div key={v.name} className="bg-background border border-border rounded-lg p-3">
                    <p className="font-medium text-sm">{v.name}</p>
                    <p className="text-xs text-muted mt-1">{v.type}</p>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Labels */}
          {Object.keys(pod.labels).length > 0 && (
            <div className="bg-card border border-border rounded-xl p-5">
              <h2 className="text-lg font-semibold mb-4">Labels</h2>
              <div className="flex flex-wrap gap-2">
                {Object.entries(pod.labels).map(([k, v]) => (
                  <span key={k} className="px-2.5 py-1 bg-white/5 rounded-lg text-xs font-mono">
                    <span className="text-accent-blue">{k}</span>
                    <span className="text-muted">: </span>
                    <span>{v}</span>
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      ) : (
        /* Logs Tab */
        <div className="bg-card border border-border rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Logs</h2>
            <div className="flex items-center gap-3">
              {pod.containers.length > 1 && (
                <select
                  value={activeContainer}
                  onChange={(e) => {
                    setSelectedContainer(e.target.value);
                    fetchLogs(e.target.value);
                  }}
                  className="bg-background border border-border rounded-lg px-3 py-1.5 text-xs focus:outline-none"
                >
                  {pod.containers.map((c) => (
                    <option key={c.name} value={c.name}>{c.name}</option>
                  ))}
                </select>
              )}
              <button
                onClick={() => fetchLogs(activeContainer)}
                className="px-3 py-1.5 bg-accent/10 text-accent rounded-lg text-xs hover:bg-accent/20 transition-colors"
              >
                Refresh
              </button>
            </div>
          </div>
          <div className="bg-black rounded-lg p-4 max-h-[600px] overflow-auto font-mono text-xs leading-5">
            {logsLoading ? (
              <div className="text-muted">Loading logs...</div>
            ) : logs ? (
              logs.split("\n").map((line, i) => (
                <div key={i} className="hover:bg-white/[0.03]">
                  <span className="text-muted/50 select-none mr-3">{i + 1}</span>
                  {line}
                </div>
              ))
            ) : (
              <div className="text-muted">No logs available</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
