"use client";

import { useCallback, useEffect, useRef, useState, use } from "react";
import { api, getApiAt, Pod, podLogStreamUrl } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import StatusBadge from "@/components/StatusBadge";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";
import Link from "next/link";

// A followed stream is unbounded; keep only a tail window so a chatty pod
// left open cannot grow memory and render cost forever.
const maxLogLines = 5000;

function appendLogLines(current: string[], lines: string[]): string[] {
  const next = [...current, ...lines];
  return next.length > maxLogLines ? next.slice(next.length - maxLogLines) : next;
}

export default function PodDetailPage({ params }: { params: Promise<{ namespace: string; name: string }> }) {
  const { namespace, name } = use(params);
  // In time-travel mode the pod is a historical snapshot: logs cannot be
  // streamed from a past moment, so the logs tab shows a notice instead.
  // Read once at mount — ClusterScope remounts this page when the scrubber
  // commits a new moment.
  const viewingPast = getApiAt() !== null;
  const [activeTab, setActiveTab] = useState<"overview" | "logs">("overview");
  const [selectedContainer, setSelectedContainer] = useState<string>("");
  const [logs, setLogs] = useState<string[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsPaused, setLogsPaused] = useState(false);
  const logSource = useRef<EventSource | null>(null);
  const logsPausedRef = useRef(false);
  // Lines received while paused; the stream keeps flowing and the buffer is
  // appended on resume. A reconnect discards it: the replayed tail becomes
  // the authoritative content instead.
  const pausedLogs = useRef<string[]>([]);

  const fetcher = useCallback(() => api.getPod(namespace, name), [namespace, name]);
  const { data: pod, error, loading, refresh } = usePolling<Pod>(fetcher);

  const stopLogs = useCallback(() => { logSource.current?.close(); logSource.current = null; }, []);
  const streamLogs = useCallback((container?: string) => {
    stopLogs(); setLogs([]); setLogsLoading(true); setLogsPaused(false);
    logsPausedRef.current = false; pausedLogs.current = [];
    const source = new EventSource(podLogStreamUrl(namespace, name, container));
    logSource.current = source;
    // Error markers go through the pause buffer too, or a failure while
    // paused would render before the earlier lines flushed on Resume.
    const appendLine = (line: string) => {
      if (logsPausedRef.current) {
        pausedLogs.current.push(line);
        if (pausedLogs.current.length > maxLogLines) pausedLogs.current.shift();
        return;
      }
      setLogs(current => appendLogLines(current, [line]));
    };
    source.addEventListener("log", (event) => {
      appendLine(JSON.parse((event as MessageEvent).data) as string);
      setLogsLoading(false);
    });
    // Every (re)connect replays the tail, so drop displayed and pause-buffered
    // lines alike: the replay is authoritative, and keeping either would
    // duplicate replayed lines after a browser auto-reconnect. Unpause too,
    // else a reconnect while paused diverts the replay into the invisible
    // buffer and the pane sits blank until Resume.
    source.onopen = () => {
      setLogs([]);
      pausedLogs.current = [];
      logsPausedRef.current = false;
      setLogsPaused(false);
      setLogsLoading(false);
    };
    // The backend signals end-of-stream; close so EventSource does not
    // auto-reconnect and replay the tail forever. A non-eof reason means the
    // upstream read failed, not that the container stopped logging.
    source.addEventListener("end", (event) => {
      stopLogs();
      setLogsLoading(false);
      const { reason } = JSON.parse((event as MessageEvent).data) as { reason: string };
      if (reason !== "eof") appendLine("Error: log stream ended unexpectedly");
    });
    source.onerror = () => {
      setLogsLoading(false);
      // Browsers auto-reconnect established streams; CLOSED means a non-200
      // (container not started, pod gone, forbidden) that will never retry.
      if (source.readyState !== EventSource.CLOSED || logSource.current !== source) return;
      logSource.current = null;
      appendLine("Error: failed to stream logs");
    };
  }, [name, namespace, stopLogs]);
  const toggleLogsPaused = () => {
    if (logsPausedRef.current) {
      const buffered = pausedLogs.current;
      setLogs(current => appendLogLines(current, buffered));
      pausedLogs.current = [];
      logsPausedRef.current = false;
      setLogsPaused(false);
    } else {
      logsPausedRef.current = true;
      setLogsPaused(true);
    }
  };
  useEffect(() => stopLogs, [stopLogs]);

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={refresh} />;
  if (!pod) return <ErrorMessage message="Pod not found" />;

  // Multi-container pods reject log requests without an explicit container,
  // so always target a concrete container: the pod's default-container
  // annotation when valid, else the first container.
  const annotatedDefault = pod.containers.some((c) => c.name === pod.defaultContainer)
    ? pod.defaultContainer
    : "";
  const activeContainer = selectedContainer || annotatedDefault || pod.containers[0]?.name || "";

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
              // Restart whenever no stream is live (never started, ended, or
              // failed), not just when the pane is empty: after an "end" or a
              // fatal error the stale tail keeps logs.length > 0 forever.
              // Never stream in past mode: the snapshot has no live logs.
              if (tab === "logs" && !viewingPast && !logSource.current) streamLogs(activeContainer);
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
                      {c.kind !== "container" && (
                        <span className="px-2 py-0.5 bg-white/5 rounded text-xs text-muted">{c.kind}</span>
                      )}
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
      ) : viewingPast ? (
        /* Logs Tab, past mode: streaming makes no sense for a snapshot. */
        <div className="bg-card border border-border rounded-xl p-8 text-center">
          <p className="text-sm text-muted">
            Logs are unavailable while viewing past cluster state — the flight
            recorder stores resource state, not log streams. Return to live to
            tail this pod&apos;s logs.
          </p>
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
                    streamLogs(e.target.value);
                  }}
                  // Accessible name distinguishes this from the sidebar's
                  // context switcher combobox.
                  aria-label="Container"
                  className="bg-background border border-border rounded-lg px-3 py-1.5 text-xs focus:outline-none"
                >
                  {pod.containers.map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.kind === "container" ? c.name : `${c.name} (${c.kind})`}
                    </option>
                  ))}
                </select>
              )}
              <button
                onClick={toggleLogsPaused}
                className="px-3 py-1.5 bg-accent/10 text-accent rounded-lg text-xs hover:bg-accent/20 transition-colors"
              >
                {logsPaused ? "Resume" : "Pause"}
              </button>
            </div>
          </div>
          <div className="bg-black rounded-lg p-4 max-h-[600px] overflow-auto font-mono text-xs leading-5">
            {logsLoading ? (
              <div className="text-muted">Loading logs...</div>
            ) : logs.length > 0 ? (
              logs.map((line, i) => (
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
