import { afterEach, describe, expect, it, vi } from "vitest";
import { api, eventSourceUrl, podLogStreamUrl, setApiContext } from "@/lib/api";

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
