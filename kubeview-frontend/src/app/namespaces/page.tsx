"use client";

import { useCallback, useState, useMemo } from "react";
import { api, Namespace } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import StatusBadge from "@/components/StatusBadge";
import SearchInput from "@/components/SearchInput";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";

export default function NamespacesPage() {
  const [search, setSearch] = useState("");
  const fetcher = useCallback(() => api.getNamespaces(), []);
  const { data, error, loading, refresh } = usePolling<Namespace[]>(fetcher);

  const filtered = useMemo(() => {
    if (!data) return [];
    if (!search) return data;
    return data.filter((ns) => ns.name.toLowerCase().includes(search.toLowerCase()));
  }, [data, search]);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Namespaces</h1>
          <p className="text-muted text-sm mt-1">{data?.length || 0} namespaces</p>
        </div>
        <div className="w-64">
          <SearchInput value={search} onChange={setSearch} placeholder="Search namespaces..." />
        </div>
      </div>

      <div className="grid grid-cols-3 gap-4">
        {filtered.map((ns) => (
          <div key={ns.name} className="bg-card border border-border rounded-xl p-5 hover:border-accent/20 transition-colors">
            <div className="flex items-center justify-between mb-3">
              <h3 className="font-semibold">{ns.name}</h3>
              <StatusBadge status={ns.status} />
            </div>
            <div className="space-y-2 text-sm text-muted">
              <div className="flex justify-between">
                <span>Created</span>
                <span>{new Date(ns.createdAt).toLocaleDateString()}</span>
              </div>
              <div className="flex justify-between">
                <span>Age</span>
                <span>{ns.age}</span>
              </div>
            </div>
            {Object.keys(ns.labels).length > 0 && (
              <div className="mt-3 flex flex-wrap gap-1.5">
                {Object.entries(ns.labels).slice(0, 3).map(([k, v]) => (
                  <span key={k} className="px-2 py-0.5 bg-white/5 rounded text-xs text-muted truncate max-w-[200px]">
                    {k}: {v}
                  </span>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
