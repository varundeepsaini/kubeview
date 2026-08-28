"use client";

import Link from "next/link";
import React, { useCallback, useEffect, useState } from "react";
import { api, HistoryRange } from "@/lib/api";
import { useCluster } from "./ClusterProvider";
import { useTimeTravel } from "./TimeTravelProvider";

// Committing within this distance of the range's end snaps back to live:
// nobody scrubs to "4 seconds ago" on purpose.
const liveSnapMs = 10_000;
// The range end drifts forward while the tab is open; refresh it so the
// scrubber's "now" edge stays honest.
const rangeRefreshMs = 30_000;

function formatMoment(ms: number): string {
  return new Date(ms).toLocaleString();
}

// TimelineBar is the flight recorder's scrubber: drag to view the cluster as
// of any recorded moment; release to commit. It renders nothing when the
// backend has history disabled — unless a past moment is pinned: the
// "Viewing past" indicator and the LIVE escape hatch must never unmount
// while pages resolve from history.
export default function TimelineBar() {
  const { context } = useCluster();
  const { at, setAt } = useTimeTravel();
  const [range, setRange] = useState<HistoryRange | null>(null);
  // dragValue tracks the thumb during a drag so the label follows it live;
  // the moment only commits (and pages only remount) on release.
  const [dragValue, setDragValue] = useState<number | null>(null);

  const loadRange = useCallback(() => {
    // The bar is an enhancement: a failed range fetch (old backend,
    // transient error) keeps the last-known-good range rather than
    // breaking the dashboard or hiding the scrubber mid-session.
    api
      .getHistoryRange()
      .then(setRange)
      .catch(() => {});
  }, []);

  useEffect(() => {
    loadRange();
    const id = setInterval(loadRange, rangeRefreshMs);
    return () => clearInterval(id);
    // Refetch on context switch: each context records its own history.
  }, [loadRange, context]);

  const viewingPast = at !== null;

  // NaN bounds make hasRange false without a separate "missing" flag.
  const startMs =
    range?.enabled && range.start ? new Date(range.start).getTime() : NaN;
  const endMs =
    range?.enabled && range.end ? new Date(range.end).getTime() : NaN;
  const hasRange = startMs < endMs;

  if (!hasRange && !viewingPast) return null;

  const value = dragValue ?? at ?? endMs;

  const commit = (ms: number) => {
    setDragValue(null);
    setAt(ms >= endMs - liveSnapMs ? null : ms);
  };

  return (
    <div
      className={`sticky top-0 z-40 ml-56 border-b px-6 py-2.5 backdrop-blur ${
        viewingPast
          ? "bg-amber-500/10 border-amber-500/40"
          : "bg-card/80 border-border"
      }`}
      data-testid="timeline-bar"
    >
      <div className="flex items-center gap-4">
        <button
          onClick={() => {
            setDragValue(null);
            setAt(null);
          }}
          className={`shrink-0 inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors ${
            viewingPast
              ? "border-border text-muted hover:text-foreground hover:bg-white/5"
              : "border-emerald-500/30 bg-emerald-500/15 text-emerald-400"
          }`}
          title={viewingPast ? "Return to live view" : "Viewing live cluster state"}
        >
          <span
            className={`w-1.5 h-1.5 rounded-full ${
              viewingPast ? "bg-amber-400" : "bg-emerald-400 animate-pulse"
            }`}
          />
          LIVE
        </button>

        {hasRange ? (
          <input
            type="range"
            min={startMs}
            max={endMs}
            step={1000}
            value={value}
            aria-label="Timeline scrubber"
            onChange={(event) => setDragValue(Number(event.target.value))}
            onPointerUp={() => dragValue !== null && commit(dragValue)}
            onKeyUp={() => dragValue !== null && commit(dragValue)}
            className="flex-1 h-1.5 cursor-pointer appearance-none rounded-full bg-border accent-amber-400"
          />
        ) : (
          // Pinned to the past with no usable range: keep the layout (and the
          // indicator + LIVE escape around it) without a scrubber.
          <div className="flex-1" />
        )}

        <div className="shrink-0 w-52 text-right text-xs">
          {dragValue !== null ? (
            <span className="text-amber-300">{formatMoment(dragValue)}</span>
          ) : viewingPast ? (
            <span className="text-amber-300 font-medium">
              Viewing past — {formatMoment(at)}
            </span>
          ) : (
            <span className="text-muted">
              history since {formatMoment(startMs)}
            </span>
          )}
        </div>

        <Link
          href="/timeline"
          className="shrink-0 text-xs text-muted hover:text-accent transition-colors"
        >
          Compare →
        </Link>
      </div>
    </div>
  );
}
