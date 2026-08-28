import { Suspense } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PodDetailPage from "@/app/pods/[namespace]/[name]/page";
import type { Pod } from "@/lib/api";

// getApiAt drives the page's past-mode guard; tests flip it per case.
let pinnedAt: number | null = null;

vi.mock("@/lib/api", () => ({
  api: {
    getPod: vi.fn(),
  },
  getApiAt: () => pinnedAt,
  podLogStreamUrl: () => "http://test/api/pods/default/web/logs",
}));

import { api } from "@/lib/api";

const getPod = vi.mocked(api.getPod);

// TrackingEventSource counts constructions: in past mode the logs tab must
// never open a stream.
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

const pod: Pod = {
  name: "web",
  namespace: "default",
  status: "Running",
  ready: "1/1",
  restarts: 0,
  node: "n1",
  ip: "10.0.0.1",
  labels: {},
  createdAt: "2026-08-27T09:00:00Z",
  age: "3h",
  containers: [
    {
      name: "app",
      image: "nginx:1.27",
      kind: "container",
      ports: [],
      ready: true,
      state: "Running",
      restartCount: 0,
    },
  ],
  conditions: [],
  volumes: [],
  defaultContainer: "",
};

// resolvedParams pre-instruments the promise with React's internal
// fulfilled-state fields so use(params) reads it synchronously instead of
// suspending — a plain resolved promise would leave the Suspense fallback up
// for the whole test.
function resolvedParams(value: { namespace: string; name: string }) {
  const params = Promise.resolve(value) as Promise<typeof value> & {
    status: string;
    value: typeof value;
  };
  params.status = "fulfilled";
  params.value = value;
  return params;
}

function renderPodDetail() {
  return render(
    <Suspense fallback={null}>
      <PodDetailPage
        params={resolvedParams({ namespace: "default", name: "web" })}
      />
    </Suspense>
  );
}

beforeEach(() => {
  vi.stubGlobal("EventSource", TrackingEventSource);
  TrackingEventSource.instances = [];
  getPod.mockResolvedValue(pod);
  pinnedAt = null;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("pod detail logs tab", () => {
  it("streams logs while live", async () => {
    renderPodDetail();

    fireEvent.click(await screen.findByRole("button", { name: "Logs" }));
    expect(TrackingEventSource.instances).toHaveLength(1);
  });

  it("shows a notice and never opens a stream in past mode", async () => {
    pinnedAt = 1_756_300_000_000;
    renderPodDetail();

    fireEvent.click(await screen.findByRole("button", { name: "Logs" }));

    expect(
      await screen.findByText(/Logs are unavailable while viewing past/)
    ).toBeInTheDocument();
    expect(TrackingEventSource.instances).toHaveLength(0);
  });
});
