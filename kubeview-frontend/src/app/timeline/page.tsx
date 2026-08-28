"use client";

import { useCallback, useEffect, useState } from "react";
import { api, HistoryDiff, HistoryRange } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";
import LoadingSpinner from "@/components/LoadingSpinner";
import ErrorMessage from "@/components/ErrorMessage";

const presets = [
  { label: "Last 15m", ms: 15 * 60 * 1000 },
  { label: "Last hour", ms: 60 * 60 * 1000 },
  { label: "Last 6h", ms: 6 * 60 * 60 * 1000 },
];

// toLocalInput renders a ms timestamp as a datetime-local input value
// (local time, minute precision).
function toLocalInput(ms: number): string {
  const date = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function fromLocalInput(value: string): number | null {
  const ms = new Date(value).getTime();
  return Number.isNaN(ms) ? null : ms;
}

// EventTypeBadge mirrors the one on the events page for the activity feed.
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

// TimelinePage is the flight recorder's diff view: pick two moments and see
// what changed in between — pods added/removed, image changes, replica
// scaling, condition transitions — plus the events that fired in the window.
export default function TimelinePage() {
  const [range, setRange] = useState<HistoryRange | null>(null);
  const [rangeLoading, setRangeLoading] = useState(true);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [diff, setDiff] = useState<HistoryDiff | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [comparing, setComparing] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .getHistoryRange()
      .then((result) => {
        if (cancelled) return;
        setRange(result);
        const end = result.end ? new Date(result.end).getTime() : Date.now();
        const start = result.start
          ? new Date(result.start).getTime()
          : end - presets[1].ms;
        setFrom(toLocalInput(Math.max(start, end - presets[1].ms)));
        setTo(toLocalInput(end));
      })
      .catch(() => {
        if (!cancelled) setRange(null);
      })
      .finally(() => {
        if (!cancelled) setRangeLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const compare = useCallback(async (fromMs: number, toMs: number) => {
    setComparing(true);
    setError(null);
    try {
      setDiff(await api.getHistoryDiff(fromMs, toMs));
    } catch (err) {
      setDiff(null);
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setComparing(false);
    }
  }, []);

  const onCompare = () => {
    const fromMs = fromLocalInput(from);
    const toMs = fromLocalInput(to);
    if (fromMs === null || toMs === null) {
      setError("Pick two valid moments to compare.");
      return;
    }
    if (fromMs >= toMs) {
      setError("The start of the window must be before its end.");
      return;
    }
    compare(fromMs, toMs);
  };

  const onPreset = (windowMs: number) => {
    const now = Date.now();
    setFrom(toLocalInput(now - windowMs));
    setTo(toLocalInput(now));
    compare(now - windowMs, now);
  };

  if (rangeLoading) return <LoadingSpinner />;

  if (!range?.enabled) {
    return (
      <div>
        <h1 className="text-2xl font-bold mb-2">Timeline</h1>
        <p className="text-muted text-sm">
          History recording is disabled on the backend
          (HISTORY_RETENTION_HOURS=0), so there is nothing to compare.
        </p>
      </div>
    );
  }

  const counts = diff
    ? {
        added: diff.changes.filter((c) => c.type === "added").length,
        removed: diff.changes.filter((c) => c.type === "removed").length,
        modified: diff.changes.filter((c) => c.type === "modified").length,
      }
    : null;

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-bold">Timeline</h1>
        <p className="text-muted text-sm mt-1">
          Compare cluster state between two moments
          {range.start && ` · history since ${new Date(range.start).toLocaleString()}`}
          {range.retentionHours && ` · ${range.retentionHours}h retention`}
        </p>
      </div>

      <div className="bg-card border border-border rounded-xl p-4 mb-6">
        <div className="flex flex-wrap items-end gap-4">
          <label className="text-xs text-muted">
            From
            <input
              type="datetime-local"
              value={from}
              min={range.start ? toLocalInput(new Date(range.start).getTime()) : undefined}
              onChange={(event) => setFrom(event.target.value)}
              className="block mt-1 bg-background border border-border rounded-lg px-3 py-2 text-sm text-foreground"
            />
          </label>
          <label className="text-xs text-muted">
            To
            <input
              type="datetime-local"
              value={to}
              onChange={(event) => setTo(event.target.value)}
              className="block mt-1 bg-background border border-border rounded-lg px-3 py-2 text-sm text-foreground"
            />
          </label>
          <button
            onClick={onCompare}
            disabled={comparing}
            className="px-4 py-2 rounded-lg bg-accent/10 text-accent border border-accent/30 text-sm font-medium hover:bg-accent/20 transition-colors disabled:opacity-50"
          >
            {comparing ? "Comparing..." : "Compare"}
          </button>
          <div className="flex items-center gap-2 ml-auto">
            {presets.map((preset) => (
              <button
                key={preset.label}
                onClick={() => onPreset(preset.ms)}
                className="px-3 py-2 rounded-lg border border-border text-xs text-muted hover:text-foreground hover:bg-white/5 transition-colors"
              >
                {preset.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {error && <ErrorMessage message={error} onRetry={onCompare} />}

      {diff && counts && (
        <>
          <p className="text-sm text-muted mb-4">
            {new Date(diff.from).toLocaleString()} →{" "}
            {new Date(diff.to).toLocaleString()} ·{" "}
            <span className="text-emerald-400">{counts.added} added</span> ·{" "}
            <span className="text-red-400">{counts.removed} removed</span> ·{" "}
            <span className="text-yellow-400">{counts.modified} modified</span>
          </p>

          <div className="bg-card border border-border rounded-xl overflow-hidden mb-6">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted text-xs border-b border-border bg-white/[0.02]">
                    <th className="text-left p-4 font-medium">Change</th>
                    <th className="text-left p-4 font-medium">Resource</th>
                    <th className="text-left p-4 font-medium">Object</th>
                    <th className="text-left p-4 font-medium">What changed</th>
                  </tr>
                </thead>
                <tbody>
                  {diff.changes.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="p-8 text-center text-muted">
                        Nothing changed in this window.
                      </td>
                    </tr>
                  ) : (
                    diff.changes.map((change) => (
                      <tr
                        key={`${change.resource}-${change.key}`}
                        className="border-b border-border/50 hover:bg-white/[0.02] align-top"
                      >
                        <td className="p-4">
                          <StatusBadge status={change.type} />
                        </td>
                        <td className="p-4 text-muted">{change.resource}</td>
                        <td className="p-4 font-mono text-xs">{change.key}</td>
                        <td className="p-4 text-muted text-xs">
                          {change.summary.length > 0 ? (
                            <ul className="space-y-0.5 font-mono">
                              {change.summary.map((line) => (
                                <li key={line}>{line}</li>
                              ))}
                            </ul>
                          ) : (
                            "—"
                          )}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          <h2 className="text-lg font-semibold mb-3">
            Events in this window{" "}
            <span className="text-muted text-sm font-normal">
              ({diff.events.length})
            </span>
          </h2>
          <div className="bg-card border border-border rounded-xl overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted text-xs border-b border-border bg-white/[0.02]">
                    <th className="text-left p-4 font-medium">Type</th>
                    <th className="text-left p-4 font-medium">Reason</th>
                    <th className="text-left p-4 font-medium">Object</th>
                    <th className="text-left p-4 font-medium">Message</th>
                    <th className="text-left p-4 font-medium">Last Seen</th>
                  </tr>
                </thead>
                <tbody>
                  {diff.events.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="p-8 text-center text-muted">
                        No events fired in this window.
                      </td>
                    </tr>
                  ) : (
                    diff.events.map((event, index) => (
                      <tr
                        key={`${event.object}-${event.reason}-${index}`}
                        className="border-b border-border/50 hover:bg-white/[0.02]"
                      >
                        <td className="p-4">
                          <EventTypeBadge type={event.type} />
                        </td>
                        <td className="p-4 font-medium">{event.reason}</td>
                        <td className="p-4 text-muted text-xs font-mono">{event.object}</td>
                        <td className="p-4 max-w-md">
                          <span className="line-clamp-2 text-muted" title={event.message}>
                            {event.message}
                          </span>
                        </td>
                        <td className="p-4 text-muted text-xs">
                          {event.lastSeen ? new Date(event.lastSeen).toLocaleString() : "—"}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}

      {!diff && !error && (
        <p className="text-muted text-sm">
          Pick a window and hit Compare — or use a preset — to see what
          changed.
        </p>
      )}
    </div>
  );
}
