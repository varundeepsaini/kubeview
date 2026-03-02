"use client";

import { useCallback, useState, useMemo } from "react";
import { api, Deployment } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import NamespaceFilter from "@/components/NamespaceFilter";
import SearchInput from "@/components/SearchInput";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";

export default function DeploymentsPage() {
  const [namespace, setNamespace] = useState("");
  const [search, setSearch] = useState("");
  const fetcher = useCallback(() => api.getDeployments(namespace || undefined), [namespace]);
  const { data, error, loading, refresh } = usePolling<Deployment[]>(fetcher);

  const filtered = useMemo(() => {
    if (!data) return [];
    if (!search) return data;
    const q = search.toLowerCase();
    return data.filter((d) => d.name.toLowerCase().includes(q) || d.namespace.toLowerCase().includes(q));
  }, [data, search]);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Deployments</h1>
          <p className="text-muted text-sm mt-1">{data?.length || 0} deployments</p>
        </div>
        <div className="flex items-center gap-3">
          <SearchInput value={search} onChange={setSearch} placeholder="Search deployments..." />
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
                <th className="text-left p-4 font-medium">Replicas</th>
                <th className="text-left p-4 font-medium">Ready</th>
                <th className="text-left p-4 font-medium">Up-to-date</th>
                <th className="text-left p-4 font-medium">Available</th>
                <th className="text-left p-4 font-medium">Strategy</th>
                <th className="text-left p-4 font-medium">Images</th>
                <th className="text-left p-4 font-medium">Age</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((dep) => {
                const healthy = dep.readyReplicas === dep.desiredReplicas;
                return (
                  <tr key={`${dep.namespace}/${dep.name}`} className="border-b border-border/50 hover:bg-white/[0.02]">
                    <td className="p-4 font-medium">{dep.name}</td>
                    <td className="p-4 text-muted">{dep.namespace}</td>
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        <div className="w-16 bg-border rounded-full h-1.5">
                          <div
                            className={`rounded-full h-1.5 ${healthy ? "bg-accent" : "bg-accent-orange"}`}
                            style={{ width: `${dep.desiredReplicas > 0 ? (dep.readyReplicas / dep.desiredReplicas) * 100 : 0}%` }}
                          />
                        </div>
                        <span className="font-mono text-xs">{dep.readyReplicas}/{dep.desiredReplicas}</span>
                      </div>
                    </td>
                    <td className="p-4 font-mono text-xs">{dep.readyReplicas}</td>
                    <td className="p-4 font-mono text-xs">{dep.updatedReplicas}</td>
                    <td className="p-4 font-mono text-xs">{dep.availableReplicas}</td>
                    <td className="p-4 text-xs text-muted">{dep.strategy}</td>
                    <td className="p-4 text-xs text-muted max-w-[200px] truncate">{dep.images.join(", ")}</td>
                    <td className="p-4 text-muted">{dep.age}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
