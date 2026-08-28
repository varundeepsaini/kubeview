import { afterEach, describe, expect, it, vi } from "vitest";
import {
  api,
  eventSourceUrl,
  HistoryResources,
  podLogStreamUrl,
  setApiAt,
  setApiContext,
} from "@/lib/api";

const BASE = "http://localhost:5501/api";

function mockFetch(response: { ok: boolean; status?: number; json: () => Promise<unknown> }) {
  const fn = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", fn);
  return fn;
}

function okFetch(data: unknown) {
  return mockFetch({ ok: true, json: async () => data });
}

afterEach(() => {
  vi.unstubAllGlobals();
  setApiContext("");
  setApiAt(null);
});

describe("fetchApi", () => {
  it("returns parsed JSON on success and disables caching", async () => {
    const cluster = { version: "v1.31.0", nodeCount: 3 };
    const fetchMock = okFetch(cluster);

    await expect(api.getCluster()).resolves.toEqual(cluster);
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/cluster`, { cache: "no-store" });
  });

  it("throws the backend error message on non-ok responses with a JSON error body", async () => {
    mockFetch({ ok: false, status: 403, json: async () => ({ error: "namespaces is forbidden" }) });

    await expect(api.getNamespaces()).rejects.toThrow("namespaces is forbidden");
  });

  it("falls back to a status-based message when the error body is not JSON", async () => {
    mockFetch({
      ok: false,
      status: 502,
      json: async () => {
        throw new SyntaxError("Unexpected token < in JSON");
      },
    });

    await expect(api.getPods()).rejects.toThrow("API error: 502");
  });

  it("falls back to a status-based message when the error body has no error field", async () => {
    mockFetch({ ok: false, status: 500, json: async () => ({ detail: "oops" }) });

    await expect(api.getServices()).rejects.toThrow("API error: 500");
  });
});

describe("URL construction", () => {
  it("omits the namespace query param when no namespace is given", async () => {
    const fetchMock = okFetch([]);
    await api.getPods();
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/pods`, { cache: "no-store" });
  });

  it("appends the namespace query param when given", async () => {
    const fetchMock = okFetch([]);
    await api.getPods("kube-system");
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/pods?namespace=kube-system`, { cache: "no-store" });
  });

  const newEndpoints = [
    ["getConfigMaps", "configmaps"],
    ["getSecrets", "secrets"],
    ["getIngresses", "ingresses"],
    ["getStatefulSets", "statefulsets"],
    ["getDaemonSets", "daemonsets"],
  ] as const;

  it.each(newEndpoints)("%s hits /%s without a namespace", async (method, path) => {
    const fetchMock = okFetch([]);
    await api[method]();
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/${path}`, { cache: "no-store" });
  });

  it.each(newEndpoints)("%s URL-encodes the namespace", async (method, path) => {
    const fetchMock = okFetch([]);
    await api[method]("team a/b&c");
    expect(fetchMock).toHaveBeenCalledWith(
      `${BASE}/${path}?namespace=team%20a%2Fb%26c`,
      { cache: "no-store" }
    );
  });
});

describe("context propagation", () => {
  it("appends the active context to every request, respecting existing query strings", async () => {
    setApiContext("prod cluster");
    const fetchMock = okFetch([]);

    await api.getNamespaces();
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/namespaces?context=prod%20cluster`, { cache: "no-store" });

    await api.getPods("kube-system");
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/pods?namespace=kube-system&context=prod%20cluster`, { cache: "no-store" });
  });

  it("stops sending a context after it is cleared", async () => {
    setApiContext("prod");
    setApiContext("");
    const fetchMock = okFetch([]);

    await api.getPods();
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/pods`, { cache: "no-store" });
  });

  it("carries the context into stream URLs", () => {
    expect(eventSourceUrl("/watch?resources=pods")).toBe(`${BASE}/watch?resources=pods`);

    setApiContext("prod");
    expect(eventSourceUrl("/watch?resources=pods")).toBe(`${BASE}/watch?resources=pods&context=prod`);
  });

  it("builds a following pod log stream URL with encoded segments", () => {
    expect(podLogStreamUrl("team a", "web/1", "app")).toBe(
      `${BASE}/pods/team%20a/web%2F1/logs?follow=true&tailLines=100&container=app`
    );
  });
});

// emptyResources builds a full /history/state resources map; tests override
// only the kinds they care about.
function historyResources(overrides: Partial<HistoryResources> = {}): HistoryResources {
  return {
    pods: [],
    deployments: [],
    services: [],
    nodes: [],
    namespaces: [],
    events: [],
    configmaps: [],
    secrets: [],
    ingresses: [],
    statefulsets: [],
    daemonsets: [],
    ...overrides,
  };
}

function historyPod(name: string, namespace: string) {
  return {
    name,
    namespace,
    status: "Running",
    ready: "1/1",
    restarts: 0,
    node: "n1",
    ip: "10.0.0.1",
    labels: {},
    createdAt: "2026-08-27T09:00:00Z",
    age: "3h",
    containers: [],
    conditions: [],
    volumes: [],
    defaultContainer: "",
  };
}

describe("time travel", () => {
  // Distinct at values per test keep the module-level snapshot cache from
  // leaking between tests.
  it("resolves list getters from /history/state when a moment is pinned", async () => {
    setApiAt(1_000_001);
    const pods = [historyPod("web", "default"), historyPod("db", "kube-system")];
    const fetchMock = okFetch({ at: "x", resources: historyResources({ pods }) });

    await expect(api.getPods()).resolves.toEqual(pods);
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/history/state?at=1000001`, { cache: "no-store" });
  });

  it("filters the snapshot by namespace client-side", async () => {
    setApiAt(1_000_002);
    const pods = [historyPod("web", "default"), historyPod("db", "kube-system")];
    okFetch({ at: "x", resources: historyResources({ pods }) });

    await expect(api.getPods("kube-system")).resolves.toEqual([pods[1]]);
  });

  it("shares one snapshot fetch across every kind", async () => {
    setApiAt(1_000_003);
    const fetchMock = okFetch({ at: "x", resources: historyResources() });

    await api.getPods();
    await api.getDeployments();
    await api.getNodes();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("resolves a single pod from the snapshot and rejects when absent", async () => {
    setApiAt(1_000_004);
    const pods = [historyPod("web", "default")];
    okFetch({ at: "x", resources: historyResources({ pods }) });

    await expect(api.getPod("default", "web")).resolves.toEqual(pods[0]);
    await expect(api.getPod("default", "gone")).rejects.toThrow(
      "Pod not found at this moment in history"
    );
  });

  it("does not cache a failed snapshot", async () => {
    setApiAt(1_000_005);
    const fetchMock = mockFetch({ ok: false, status: 500, json: async () => ({}) });

    await expect(api.getPods()).rejects.toThrow("API error: 500");
    await expect(api.getPods()).rejects.toThrow("API error: 500");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("refetches the snapshot when the pinned moment changes", async () => {
    setApiAt(1_000_008);
    const fetchMock = okFetch({ at: "x", resources: historyResources() });

    await api.getPods();
    setApiAt(1_000_009);
    await api.getPods();

    // Scrubbing to a new moment must not serve the old snapshot.
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenLastCalledWith(
      `${BASE}/history/state?at=1000009`,
      { cache: "no-store" }
    );
  });

  it("returns to live endpoints once the moment clears", async () => {
    setApiAt(1_000_006);
    setApiAt(null);
    const fetchMock = okFetch([]);

    await api.getPods();
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/pods`, { cache: "no-store" });
  });

  it("hits the history range and diff endpoints", async () => {
    const fetchMock = okFetch({ enabled: true });

    await api.getHistoryRange();
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/history/range`, { cache: "no-store" });

    await api.getHistoryDiff(1000, 2000);
    expect(fetchMock).toHaveBeenCalledWith(`${BASE}/history/diff?from=1000&to=2000`, { cache: "no-store" });
  });

  it("carries the active context into snapshot fetches", async () => {
    setApiContext("prod");
    setApiAt(1_000_007);
    const fetchMock = okFetch({ at: "x", resources: historyResources() });

    await api.getPods();
    expect(fetchMock).toHaveBeenCalledWith(
      `${BASE}/history/state?at=1000007&context=prod`,
      { cache: "no-store" }
    );
  });
});
