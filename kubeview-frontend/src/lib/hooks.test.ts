import { renderHook, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useNow, usePolling, useWatchList } from "@/lib/hooks";

vi.mock("@/lib/api", () => ({
  getApiAt: vi.fn(() => null),
  eventSourceUrl: (path: string) => `http://test/api${path}`,
}));

import { getApiAt } from "@/lib/api";

const getApiAtMock = vi.mocked(getApiAt);

// TrackingEventSource counts constructions so tests can assert whether a
// hook opened a live watch stream at all.
class TrackingEventSource {
  static instances: TrackingEventSource[] = [];
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  readyState = 0;
  onopen: ((ev: Event) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  constructor(public url: string) {
    TrackingEventSource.instances.push(this);
  }
  addEventListener() {}
  close() {
    this.readyState = TrackingEventSource.CLOSED;
  }
}

beforeEach(() => {
  vi.stubGlobal("EventSource", TrackingEventSource);
  TrackingEventSource.instances = [];
  getApiAtMock.mockReturnValue(null);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const pinnedMoment = 1_756_300_000_000;

describe("usePolling", () => {
  it("refetches on the interval while live", async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn().mockResolvedValue("data");
    const { unmount } = renderHook(() => usePolling(fetcher, 5000));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetcher).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(fetcher).toHaveBeenCalledTimes(2);
    unmount();
  });

  it("fetches the frozen snapshot exactly once in past mode", async () => {
    getApiAtMock.mockReturnValue(pinnedMoment);
    vi.useFakeTimers();
    const fetcher = vi.fn().mockResolvedValue("snapshot");
    const { result, unmount } = renderHook(() => usePolling(fetcher, 5000));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(20_000);
    });
    // A paused cluster must not tick: one fetch, no matter how long the
    // page stays open.
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.data).toBe("snapshot");
    unmount();
  });
});

describe("useWatchList", () => {
  it("opens one shared watch stream while live", async () => {
    const fetcher = vi.fn().mockResolvedValue([]);
    const { unmount } = renderHook(() => useWatchList(fetcher, "pods"));

    // subscribeWatch coalesces into a microtask; flush it.
    await act(async () => {});
    expect(TrackingEventSource.instances).toHaveLength(1);
    expect(TrackingEventSource.instances[0].url).toContain("resources=pods");

    unmount();
    await act(async () => {});
  });

  it("opens no watch stream in past mode and fetches once", async () => {
    getApiAtMock.mockReturnValue(pinnedMoment);
    vi.useFakeTimers();
    const fetcher = vi.fn().mockResolvedValue([]);
    const { unmount } = renderHook(() => useWatchList(fetcher, "pods"));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    // The snapshot is frozen: no SSE connection, no refetch churn.
    expect(TrackingEventSource.instances).toHaveLength(0);
    expect(fetcher).toHaveBeenCalledTimes(1);
    unmount();
  });
});

describe("useNow", () => {
  it("ticks with the wall clock while live", () => {
    vi.useFakeTimers();
    vi.setSystemTime(5_000_000);
    const { result, unmount } = renderHook(() => useNow());

    expect(result.current).toBe(5_000_000);
    act(() => {
      vi.advanceTimersByTime(30_000);
    });
    expect(result.current).toBe(5_030_000);
    unmount();
  });

  it("freezes at the pinned moment in past mode", () => {
    getApiAtMock.mockReturnValue(pinnedMoment);
    vi.useFakeTimers();
    vi.setSystemTime(5_000_000);
    const { result, unmount } = renderHook(() => useNow());

    // Ages must read relative to the viewed moment, not the wall clock —
    // and must never advance while paused.
    expect(result.current).toBe(pinnedMoment);
    act(() => {
      vi.advanceTimersByTime(120_000);
    });
    expect(result.current).toBe(pinnedMoment);
    unmount();
  });
});
