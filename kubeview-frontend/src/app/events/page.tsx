"use client";

import { useCallback, useState, useMemo } from "react";
import { api, KubeEvent } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import NamespaceFilter from "@/components/NamespaceFilter";
import SearchInput from "@/components/SearchInput";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";

function formatRelative(timestamp: string): string {
  if (!timestamp) return "—";
  const created = new Date(timestamp).getTime();
  if (Number.isNaN(created)) return "—";
  const diffSecs = Math.floor((Date.now() - created) / 1000);
  if (diffSecs < 60) return `${diffSecs}s ago`;
  const diffMins = Math.floor(diffSecs / 60);
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ago`;
}

function EventTypeBadge({ type }: { type: string }) {
  const isWarning = type === "Warning";
  const colors = isWarning
    ? "bg-orange-500/15 text-orange-400 border-orange-500/30"
    : "bg-emerald-500/15 text-emerald-400 border-emerald-500/30";
  const dot = isWarning ? "bg-orange-400" : "bg-emerald-400";
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border font-medium px-2 py-0.5 text-xs ${colors}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${dot}`} />
      {type}
    </span>
  );
}

export default function EventsPage() {
  const [namespace, setNamespace] = useState("");
  const [search, setSearch] = useState("");
  const fetcher = useCallback(() => api.getEvents(namespace || undefined), [namespace]);
  const { data, error, loading, refresh } = usePolling<KubeEvent[]>(fetcher);

  const sortedAndFiltered = useMemo(() => {
    if (!data) return [];
    const filtered = !search
      ? data
      : data.filter((e) => {
          const q = search.toLowerCase();
          return (
            e.reason.toLowerCase().includes(q) ||
            e.object.toLowerCase().includes(q) ||
            e.message.toLowerCase().includes(q)
          );
        });
    return [...filtered].sort((a, b) => {
      const aTime = new Date(a.lastSeen).getTime() || 0;
      const bTime = new Date(b.lastSeen).getTime() || 0;
      return bTime - aTime;
    });
  }, [data, search]);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;

  const warningCount = data?.filter((e) => e.type === "Warning").length || 0;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Events</h1>
          <p className="text-muted text-sm mt-1">
            {data?.length || 0} events
            {warningCount > 0 && (
              <span className="text-orange-400"> · {warningCount} warning{warningCount === 1 ? "" : "s"}</span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <SearchInput value={search} onChange={setSearch} placeholder="Search events..." />
          <NamespaceFilter value={namespace} onChange={setNamespace} />
        </div>
      </div>

      <div className="bg-card border border-border rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted text-xs border-b border-border bg-white/[0.02]">
                <th className="text-left p-4 font-medium">Type</th>
                <th className="text-left p-4 font-medium">Reason</th>
                <th className="text-left p-4 font-medium">Object</th>
                <th className="text-left p-4 font-medium">Namespace</th>
                <th className="text-left p-4 font-medium">Message</th>
                <th className="text-left p-4 font-medium">Count</th>
                <th className="text-left p-4 font-medium">Source</th>
                <th className="text-left p-4 font-medium">Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {sortedAndFiltered.length === 0 ? (
                <tr>
                  <td colSpan={8} className="p-8 text-center text-muted">
                    No events to show.
                  </td>
                </tr>
              ) : (
                sortedAndFiltered.map((e, i) => (
                  <tr
                    key={`${e.object}-${e.reason}-${e.lastSeen}-${i}`}
                    className="border-b border-border/50 hover:bg-white/[0.02]"
                  >
                    <td className="p-4">
                      <EventTypeBadge type={e.type} />
                    </td>
                    <td className="p-4 font-medium">{e.reason}</td>
                    <td className="p-4 text-muted text-xs font-mono">{e.object}</td>
                    <td className="p-4 text-muted">{e.namespace}</td>
                    <td className="p-4 max-w-md">
                      <span className="line-clamp-2 text-muted" title={e.message}>
                        {e.message}
                      </span>
                    </td>
                    <td className="p-4">
                      {e.count > 1 ? (
                        <span className="text-accent-orange">{e.count}</span>
                      ) : (
                        e.count
                      )}
                    </td>
                    <td className="p-4 text-muted text-xs">{e.source}</td>
                    <td className="p-4 text-muted">{formatRelative(e.lastSeen)}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
