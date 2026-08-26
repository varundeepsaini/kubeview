"use client";

import { ReactNode, useCallback, useMemo, useState } from "react";
import { usePolling } from "@/lib/hooks";
import NamespaceFilter from "./NamespaceFilter";
import SearchInput from "./SearchInput";
import LoadingSpinner from "./LoadingSpinner";
import ErrorMessage from "./ErrorMessage";

export interface NamedResource { name: string; namespace: string }
export interface ResourceColumn<T> { label: string; render: (item: T) => ReactNode }

export default function ResourceList<T extends NamedResource>({ title, fetcher, columns }: { title: string; fetcher: (namespace?: string) => Promise<T[]>; columns: ResourceColumn<T>[] }) {
  const [namespace, setNamespace] = useState("");
  const [search, setSearch] = useState("");
  const load = useCallback(() => fetcher(namespace || undefined), [fetcher, namespace]);
  const { data, error, loading, refresh } = usePolling(load);
  const filtered = useMemo(() => (data ?? []).filter((item) => `${item.name} ${item.namespace}`.toLowerCase().includes(search.toLowerCase())), [data, search]);
  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;
  return <div>
    <div className="flex items-center justify-between gap-4 mb-6"><div><h1 className="text-2xl font-bold">{title}</h1><p className="text-muted text-sm mt-1">{data?.length ?? 0} {title.toLowerCase()}</p></div><div className="flex items-center gap-3"><SearchInput value={search} onChange={setSearch} placeholder={`Search ${title.toLowerCase()}...`} /><NamespaceFilter value={namespace} onChange={setNamespace} /></div></div>
    <div className="bg-card border border-border rounded-lg overflow-hidden"><div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="text-muted text-xs border-b border-border bg-white/[0.02]"><th className="text-left p-4 font-medium">Name</th><th className="text-left p-4 font-medium">Namespace</th>{columns.map(c => <th key={c.label} className="text-left p-4 font-medium">{c.label}</th>)}</tr></thead><tbody>{filtered.map(item => <tr key={`${item.namespace}/${item.name}`} className="border-b border-border/50 hover:bg-white/[0.02]"><td className="p-4 font-medium">{item.name}</td><td className="p-4 text-muted">{item.namespace}</td>{columns.map(c => <td key={c.label} className="p-4 text-muted">{c.render(item)}</td>)}</tr>)}</tbody></table></div></div>
  </div>;
}
