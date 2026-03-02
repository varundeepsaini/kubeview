"use client";

import { useCallback } from "react";
import { api, Namespace } from "@/lib/api";
import { usePolling } from "@/lib/hooks";

interface NamespaceFilterProps {
  value: string;
  onChange: (ns: string) => void;
}

export default function NamespaceFilter({ value, onChange }: NamespaceFilterProps) {
  const fetcher = useCallback(() => api.getNamespaces(), []);
  const { data: namespaces } = usePolling<Namespace[]>(fetcher, 30000);

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="bg-card border border-border rounded-lg px-3 py-2 text-sm text-foreground focus:outline-none focus:border-accent/50"
    >
      <option value="">All Namespaces</option>
      {namespaces?.map((ns) => (
        <option key={ns.name} value={ns.name}>
          {ns.name}
        </option>
      ))}
    </select>
  );
}
