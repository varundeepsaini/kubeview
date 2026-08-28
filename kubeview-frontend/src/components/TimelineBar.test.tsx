import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TimelineBar from "@/components/TimelineBar";
import { ClusterProvider, useCluster } from "@/components/ClusterProvider";
import {
  TimeTravelProvider,
  useTimeTravel,
} from "@/components/TimeTravelProvider";

vi.mock("@/lib/api", () => ({
  api: {
    getHistoryRange: vi.fn(),
  },
  setApiContext: vi.fn(),
  setApiAt: vi.fn(),
}));

import { api, setApiAt } from "@/lib/api";

const getHistoryRange = vi.mocked(api.getHistoryRange);
const setApiAtMock = vi.mocked(setApiAt);

const rangeStart = "2026-08-27T09:00:00Z";
const rangeEnd = "2026-08-27T12:00:00Z";
const startMs = new Date(rangeStart).getTime();
const endMs = new Date(rangeEnd).getTime();

// PinPast pins a past moment from inside the providers, standing in for a
// scrub committed before the range became unavailable.
function PinPast({ at }: { at: number }) {
  const { setAt } = useTimeTravel();
  useEffect(() => {
    setAt(at);
  }, [at, setAt]);
  return null;
}

// SwitchContext exposes a cluster switch, like the sidebar dropdown does.
function SwitchContext() {
  const { setContext } = useCluster();
  return (
    <button type="button" onClick={() => setContext("other")}>
      switch context
    </button>
  );
}

function renderBar(extras?: React.ReactNode) {
  return render(
    <ClusterProvider>
      <TimeTravelProvider>
        {extras}
        <TimelineBar />
      </TimeTravelProvider>
    </ClusterProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe("TimelineBar", () => {
  it("renders nothing while history is disabled", async () => {
    getHistoryRange.mockResolvedValue({ enabled: false });
    renderBar();

    await waitFor(() => expect(getHistoryRange).toHaveBeenCalled());
    expect(screen.queryByTestId("timeline-bar")).not.toBeInTheDocument();
  });

  it("renders nothing when the range fetch fails", async () => {
    getHistoryRange.mockRejectedValue(new Error("no backend"));
    renderBar();

    await waitFor(() => expect(getHistoryRange).toHaveBeenCalled());
    expect(screen.queryByTestId("timeline-bar")).not.toBeInTheDocument();
  });

  it("shows the scrubber with the recorded range when enabled", async () => {
    getHistoryRange.mockResolvedValue({
      enabled: true,
      start: rangeStart,
      end: rangeEnd,
      retentionHours: 72,
    });
    renderBar();

    const slider = await screen.findByRole("slider", { name: "Timeline scrubber" });
    expect(slider).toHaveAttribute("min", String(startMs));
    expect(slider).toHaveAttribute("max", String(endMs));
    expect(screen.getByText(/history since/i)).toBeInTheDocument();
  });

  it("commits a scrubbed moment on release and returns to live via the LIVE button", async () => {
    getHistoryRange.mockResolvedValue({
      enabled: true,
      start: rangeStart,
      end: rangeEnd,
      retentionHours: 72,
    });
    renderBar();

    const slider = await screen.findByRole("slider", { name: "Timeline scrubber" });
    const past = startMs + 30 * 60 * 1000;
    fireEvent.change(slider, { target: { value: String(past) } });
    fireEvent.pointerUp(slider);

    expect(setApiAtMock).toHaveBeenCalledWith(past);
    expect(await screen.findByText(/Viewing past/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /LIVE/ }));
    expect(setApiAtMock).toHaveBeenLastCalledWith(null);
    await waitFor(() =>
      expect(screen.queryByText(/Viewing past/)).not.toBeInTheDocument()
    );
  });

  it("snaps back to live when releasing at the range's end", async () => {
    getHistoryRange.mockResolvedValue({
      enabled: true,
      start: rangeStart,
      end: rangeEnd,
      retentionHours: 72,
    });
    renderBar();

    const slider = await screen.findByRole("slider", { name: "Timeline scrubber" });
    fireEvent.change(slider, { target: { value: String(endMs - 1000) } });
    fireEvent.pointerUp(slider);

    expect(setApiAtMock).toHaveBeenLastCalledWith(null);
    expect(screen.queryByText(/Viewing past/)).not.toBeInTheDocument();
  });

  it("keeps the past indicator and LIVE escape when the range is unavailable", async () => {
    getHistoryRange.mockRejectedValue(new Error("no backend"));
    renderBar(<PinPast at={startMs} />);

    // Pinned to the past with no usable range: the indicator and the escape
    // hatch must render anyway (only the scrubber may be omitted).
    expect(await screen.findByText(/Viewing past/)).toBeInTheDocument();
    expect(
      screen.queryByRole("slider", { name: "Timeline scrubber" })
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /LIVE/ }));
    expect(setApiAtMock).toHaveBeenLastCalledWith(null);
    await waitFor(() =>
      expect(screen.queryByTestId("timeline-bar")).not.toBeInTheDocument()
    );
  });

  it("returns to live when the kubeconfig context changes", async () => {
    getHistoryRange.mockResolvedValue({
      enabled: true,
      start: rangeStart,
      end: rangeEnd,
      retentionHours: 72,
    });
    renderBar(<SwitchContext />);

    const slider = await screen.findByRole("slider", { name: "Timeline scrubber" });
    const past = startMs + 30 * 60 * 1000;
    fireEvent.change(slider, { target: { value: String(past) } });
    fireEvent.pointerUp(slider);
    expect(await screen.findByText(/Viewing past/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /switch context/ }));

    // A moment pinned in one context must never survive into another: the
    // provider drops back to live before any page can fetch history.
    expect(setApiAtMock).toHaveBeenLastCalledWith(null);
    await waitFor(() =>
      expect(screen.queryByText(/Viewing past/)).not.toBeInTheDocument()
    );
  });
});
