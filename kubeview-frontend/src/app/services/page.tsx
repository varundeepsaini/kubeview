"use client";

import { useCallback, useState, useMemo } from "react";
import { api, Service } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import NamespaceFilter from "@/components/NamespaceFilter";
import SearchInput from "@/components/SearchInput";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";

export default function ServicesPage() {
  const [namespace, setNamespace] = useState("");
  const [search, setSearch] = useState("");
  const fetcher = useCallback(() => api.getServices(namespace || undefined), [namespace]);
  const { data, error, loading, refresh } = usePolling<Service[]>(fetcher);

  const filtered = useMemo(() => {
    if (!data) return [];
    if (!search) return data;
    const q = search.toLowerCase();
    return data.filter((s) => s.name.toLowerCase().includes(q) || s.namespace.toLowerCase().includes(q) || s.type.toLowerCase().includes(q));
  }, [data, search]);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Services</h1>
          <p className="text-muted text-sm mt-1">{data?.length || 0} services</p>
        </div>
        <div className="flex items-center gap-3">
          <SearchInput value={search} onChange={setSearch} placeholder="Search services..." />
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
                <th className="text-left p-4 font-medium">Type</th>
                <th className="text-left p-4 font-medium">Cluster IP</th>
                <th className="text-left p-4 font-medium">External IP</th>
                <th className="text-left p-4 font-medium">Ports</th>
                <th className="text-left p-4 font-medium">Age</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((svc) => (
                <tr key={`${svc.namespace}/${svc.name}`} className="border-b border-border/50 hover:bg-white/[0.02]">
                  <td className="p-4 font-medium">{svc.name}</td>
                  <td className="p-4 text-muted">{svc.namespace}</td>
                  <td className="p-4">
                    <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                      svc.type === "ClusterIP" ? "bg-blue-500/15 text-blue-400" :
                      svc.type === "NodePort" ? "bg-purple-500/15 text-purple-400" :
                      svc.type === "LoadBalancer" ? "bg-green-500/15 text-green-400" :
                      "bg-gray-500/15 text-gray-400"
                    }`}>
                      {svc.type}
                    </span>
                  </td>
                  <td className="p-4 font-mono text-xs text-muted">{svc.clusterIp}</td>
                  <td className="p-4 font-mono text-xs text-muted">{svc.externalIp}</td>
                  <td className="p-4 font-mono text-xs text-muted">{svc.ports.join(", ")}</td>
                  <td className="p-4 text-muted">{svc.age}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
