import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TimelinePage from "@/app/timeline/page";

vi.mock("@/lib/api", () => ({
  api: {
    getHistoryRange: vi.fn(),
    getHistoryDiff: vi.fn(),
  },
}));

import { api } from "@/lib/api";

const getHistoryRange = vi.mocked(api.getHistoryRange);
const getHistoryDiff = vi.mocked(api.getHistoryDiff);

const enabledRange = {
  enabled: true,
  start: "2026-08-27T09:00:00Z",
  end: "2026-08-27T12:00:00Z",
  retentionHours: 72,
};

const sampleDiff = {
  from: "2026-08-27T10:00:00Z",
  to: "2026-08-27T11:00:00Z",
  changes: [
    {
      resource: "pods",
      key: "default/web",
      type: "modified" as const,
      summary: ["restarts: 0 → 3", "status: Running → Failed"],
    },
    {
      resource: "deployments",
      key: "default/api",
      type: "added" as const,
      summary: [],
    },
  ],
  events: [
    {
      name: "web.1",
      type: "Warning",
      reason: "BackOff",
      message: "Back-off restarting failed container",
      object: "Pod/web",
      namespace: "default",
      firstSeen: "2026-08-27T10:30:00Z",
      lastSeen: "2026-08-27T10:45:00Z",
      count: 3,
      source: "kubelet",
    },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("TimelinePage", () => {
  it("explains itself when history recording is disabled", async () => {
    getHistoryRange.mockResolvedValue({ enabled: false });
    render(<TimelinePage />);

    expect(
      await screen.findByText(/History recording is disabled/)
    ).toBeInTheDocument();
    expect(getHistoryDiff).not.toHaveBeenCalled();
  });

  it("renders change rows, summaries and events for a preset window", async () => {
    getHistoryRange.mockResolvedValue(enabledRange);
    getHistoryDiff.mockResolvedValue(sampleDiff);
    render(<TimelinePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Last hour" }));

    // Summary counts, one per change type.
    expect(await screen.findByText("1 added")).toBeInTheDocument();
    expect(screen.getByText("0 removed")).toBeInTheDocument();
    expect(screen.getByText("1 modified")).toBeInTheDocument();

    // The modified row carries its human-readable field diffs.
    expect(screen.getByText("default/web")).toBeInTheDocument();
    expect(screen.getByText("restarts: 0 → 3")).toBeInTheDocument();
    expect(screen.getByText("status: Running → Failed")).toBeInTheDocument();

    // The added row renders without a summary.
    expect(screen.getByText("default/api")).toBeInTheDocument();

    // The activity feed lists the window's events.
    expect(screen.getByText("BackOff")).toBeInTheDocument();
    expect(
      screen.getByText("Back-off restarting failed container")
    ).toBeInTheDocument();
  });

  it("rejects an inverted window without calling the backend", async () => {
    getHistoryRange.mockResolvedValue(enabledRange);
    render(<TimelinePage />);

    const from = await screen.findByLabelText("From");
    const to = screen.getByLabelText("To");
    fireEvent.change(from, { target: { value: "2026-08-27T14:00" } });
    fireEvent.change(to, { target: { value: "2026-08-27T13:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Compare" }));

    expect(
      await screen.findByText(/start of the window must be before its end/)
    ).toBeInTheDocument();
    expect(getHistoryDiff).not.toHaveBeenCalled();
  });

  it("surfaces a failed comparison with a retry path", async () => {
    getHistoryRange.mockResolvedValue(enabledRange);
    getHistoryDiff.mockRejectedValueOnce(new Error("history store closed"));
    getHistoryDiff.mockResolvedValueOnce(sampleDiff);
    render(<TimelinePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Last hour" }));
    expect(
      await screen.findByText("history store closed")
    ).toBeInTheDocument();

    // The error view's retry re-runs the same comparison.
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(getHistoryDiff).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("1 modified")).toBeInTheDocument();
  });
});
