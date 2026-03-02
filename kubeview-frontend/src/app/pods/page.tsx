"use client";

import { useCallback, useState, useMemo } from "react";
import { api, Pod } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import StatusBadge from "@/components/StatusBadge";
import NamespaceFilter from "@/components/NamespaceFilter";
import SearchInput from "@/components/SearchInput";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";
import Link from "next/link";

export default function PodsPage() {
  const [namespace, setNamespace] = useState("");
  const [search, setSearch] = useState("");
  const fetcher = useCallback(() => api.getPods(namespace || undefined), [namespace]);
  const { data, error, loading, refresh } = usePolling<Pod[]>(fetcher);

  const filtered = useMemo(() => {
    if (!data) return [];
    if (!search) return data;
    const q = search.toLowerCase();
    return data.filter(
      (p) => p.name.toLowerCase().includes(q) || p.namespace.toLowerCase().includes(q) || p.status.toLowerCase().includes(q)
    );
  }, [data, search]);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Pods</h1>
          <p className="text-muted text-sm mt-1">{data?.length || 0} pods</p>
        </div>
        <div className="flex items-center gap-3">
          <SearchInput value={search} onChange={setSearch} placeholder="Search pods..." />
          <NamespaceFilter value={namespace} onChange={setNamespace} />
        </div>
      </div>

      <div className="bg-card border border-border rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted text-xs border-b border-border bg-white/[0.02]">
                <th className="text-left p-4 font-medium">Name</th>
                <th className="text-left p-4 font-medium">Namespace</th>
                <th className="text-left p-4 font-medium">Status</th>
                <th className="text-left p-4 font-medium">Ready</th>
                <th className="text-left p-4 font-medium">Restarts</th>
                <th className="text-left p-4 font-medium">Node</th>
                <th className="text-left p-4 font-medium">IP</th>
                <th className="text-left p-4 font-medium">Age</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((pod) => (
                <tr key={`${pod.namespace}/${pod.name}`} className="border-b border-border/50 hover:bg-white/[0.02]">
                  <td className="p-4">
                    <Link href={`/pods/${pod.namespace}/${pod.name}`} className="text-accent-blue hover:underline">
                      {pod.name}
                    </Link>
                  </td>
                  <td className="p-4 text-muted">{pod.namespace}</td>
                  <td className="p-4"><StatusBadge status={pod.status} /></td>
                  <td className="p-4 font-mono text-xs">{pod.ready}</td>
                  <td className="p-4">{pod.restarts > 0 ? <span className="text-accent-orange">{pod.restarts}</span> : 0}</td>
                  <td className="p-4 text-muted text-xs">{pod.node}</td>
                  <td className="p-4 font-mono text-xs text-muted">{pod.ip}</td>
                  <td className="p-4 text-muted">{pod.age}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
