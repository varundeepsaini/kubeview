"use client";

import { useCallback } from "react";
import { api, ClusterInfo, Namespace, Pod, Deployment, NodeInfo } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import StatusBadge from "@/components/StatusBadge";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";
import Link from "next/link";

export default function Dashboard() {
  const clusterFetcher = useCallback(() => api.getCluster(), []);
  const nsFetcher = useCallback(() => api.getNamespaces(), []);
  const podsFetcher = useCallback(() => api.getPods(), []);
  const depsFetcher = useCallback(() => api.getDeployments(), []);
  const nodesFetcher = useCallback(() => api.getNodes(), []);

  const { data: cluster, error: clusterErr, loading } = usePolling<ClusterInfo>(clusterFetcher);
  const { data: namespaces } = usePolling<Namespace[]>(nsFetcher);
  const { data: pods } = usePolling<Pod[]>(podsFetcher);
  const { data: deployments } = usePolling<Deployment[]>(depsFetcher);
  const { data: nodes } = usePolling<NodeInfo[]>(nodesFetcher);

  if (loading) return <LoadingSpinner message="Connecting to cluster..." />;
  if (clusterErr) return <ErrorMessage message={clusterErr} />;

  const runningPods = pods?.filter((p) => p.status === "Running").length || 0;
  const totalPods = pods?.length || 0;
  const healthyDeps = deployments?.filter((d) => d.readyReplicas === d.desiredReplicas).length || 0;
  const totalDeps = deployments?.length || 0;
  const readyNodes = nodes?.filter((n) => n.status === "Ready").length || 0;
  const totalNodes = nodes?.length || 0;

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <p className="text-muted text-sm mt-1">
          Cluster: <span className="text-accent">{cluster?.clusterName}</span> | Version: {cluster?.version} | Platform: {cluster?.platform}
        </p>
      </div>

      {/* Bento Grid */}
      <div className="grid grid-cols-4 gap-4 mb-8">
        <Link href="/pods" className="bg-card border border-border rounded-xl p-5 hover:border-accent/30 transition-colors">
          <div className="flex items-center justify-between mb-3">
            <span className="text-muted text-sm">Pods</span>
            <svg className="w-5 h-5 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" />
            </svg>
          </div>
          <p className="text-3xl font-bold">{runningPods}<span className="text-muted text-lg font-normal">/{totalPods}</span></p>
          <p className="text-xs text-muted mt-1">Running</p>
        </Link>

        <Link href="/deployments" className="bg-card border border-border rounded-xl p-5 hover:border-accent-blue/30 transition-colors">
          <div className="flex items-center justify-between mb-3">
            <span className="text-muted text-sm">Deployments</span>
            <svg className="w-5 h-5 text-accent-blue" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6.429 9.75L2.25 12l4.179 2.25m0-4.5l5.571 3 5.571-3m-11.142 0L2.25 7.5 12 2.25l9.75 5.25-4.179 2.25m0 0L12 12.75 6.429 9.75m11.142 0l4.179 2.25-9.75 5.25-9.75-5.25 4.179-2.25" />
            </svg>
          </div>
          <p className="text-3xl font-bold">{healthyDeps}<span className="text-muted text-lg font-normal">/{totalDeps}</span></p>
          <p className="text-xs text-muted mt-1">Healthy</p>
        </Link>

        <Link href="/namespaces" className="bg-card border border-border rounded-xl p-5 hover:border-accent-purple/30 transition-colors">
          <div className="flex items-center justify-between mb-3">
            <span className="text-muted text-sm">Namespaces</span>
            <svg className="w-5 h-5 text-accent-purple" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
            </svg>
          </div>
          <p className="text-3xl font-bold">{namespaces?.length || 0}</p>
          <p className="text-xs text-muted mt-1">Active</p>
        </Link>

        <Link href="/nodes" className="bg-card border border-border rounded-xl p-5 hover:border-accent-orange/30 transition-colors">
          <div className="flex items-center justify-between mb-3">
            <span className="text-muted text-sm">Nodes</span>
            <svg className="w-5 h-5 text-accent-orange" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7" />
            </svg>
          </div>
          <p className="text-3xl font-bold">{readyNodes}<span className="text-muted text-lg font-normal">/{totalNodes}</span></p>
          <p className="text-xs text-muted mt-1">Ready</p>
        </Link>
      </div>

      {/* Recent Pods Table */}
      <div className="bg-card border border-border rounded-xl p-5 mb-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Recent Pods</h2>
          <Link href="/pods" className="text-xs text-accent hover:underline">View all</Link>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted text-xs border-b border-border">
                <th className="text-left pb-3 font-medium">Name</th>
                <th className="text-left pb-3 font-medium">Namespace</th>
                <th className="text-left pb-3 font-medium">Status</th>
                <th className="text-left pb-3 font-medium">Ready</th>
                <th className="text-left pb-3 font-medium">Restarts</th>
                <th className="text-left pb-3 font-medium">Age</th>
              </tr>
            </thead>
            <tbody>
              {pods?.slice(0, 8).map((pod) => (
                <tr key={`${pod.namespace}/${pod.name}`} className="border-b border-border/50 hover:bg-white/[0.02]">
                  <td className="py-3">
                    <Link href={`/pods/${pod.namespace}/${pod.name}`} className="text-accent-blue hover:underline">
                      {pod.name}
                    </Link>
                  </td>
                  <td className="py-3 text-muted">{pod.namespace}</td>
                  <td className="py-3"><StatusBadge status={pod.status} /></td>
                  <td className="py-3 font-mono text-xs">{pod.ready}</td>
                  <td className="py-3">{pod.restarts}</td>
                  <td className="py-3 text-muted">{pod.age}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Deployments Overview */}
      <div className="bg-card border border-border rounded-xl p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Deployments</h2>
          <Link href="/deployments" className="text-xs text-accent hover:underline">View all</Link>
        </div>
        <div className="grid grid-cols-2 gap-3">
          {deployments?.map((dep) => (
            <div key={`${dep.namespace}/${dep.name}`} className="bg-background border border-border rounded-lg p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="font-medium text-sm">{dep.name}</span>
                <span className="text-xs text-muted">{dep.namespace}</span>
              </div>
              <div className="flex items-center gap-3">
                <div className="flex-1 bg-border rounded-full h-1.5">
                  <div
                    className="bg-accent rounded-full h-1.5 transition-all"
                    style={{ width: `${dep.desiredReplicas > 0 ? (dep.readyReplicas / dep.desiredReplicas) * 100 : 0}%` }}
                  />
                </div>
                <span className="text-xs font-mono text-muted">{dep.readyReplicas}/{dep.desiredReplicas}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
