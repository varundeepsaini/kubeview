import { execSync } from "node:child_process";
import { test, expect, type Page } from "@playwright/test";

// These tests mutate the cluster, so everything lives in a dedicated
// namespace with names no other spec matches. Row changes alone would not
// prove live delivery: on any stream error the frontend falls back to
// re-listing, so a broken watch still converges via refetches. Each test
// therefore also asserts that no list request fires after the initial load
// settles, pinning the transitions to SSE watch events.

const NS = "e2e-live";
const POD = "e2e-live-pod";

const kubectl = (args: string, input?: string) =>
  execSync(`kubectl ${args}`, { encoding: "utf8", input });

// Records every list request for the given API path (subpaths like
// /api/pods/{ns}/{name} do not match) issued after the tracker is attached.
const trackListRequests = (page: Page, apiPath: string): string[] => {
  const requests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.endsWith(apiPath)) {
      requests.push(request.url());
    }
  });
  return requests;
};

const namespaceManifest = (name: string) =>
  `apiVersion: v1\nkind: Namespace\nmetadata:\n  name: ${name}\n`;

// busybox's sh does not forward SIGTERM, so without a short grace period a
// delete would sit in Terminating for 30s before the watch sees "deleted".
const podManifest = `
apiVersion: v1
kind: Pod
metadata:
  name: ${POD}
  namespace: ${NS}
spec:
  terminationGracePeriodSeconds: 1
  containers:
    - name: main
      image: busybox:1.36
      command: ["sh", "-c", "sleep 3600"]
`;

test.describe("live watch updates", () => {
  test.beforeAll(() => {
    kubectl("apply -f -", namespaceManifest(NS));
  });

  test.afterAll(() => {
    // Wait for full deletion so a CI retry's beforeAll can recreate the
    // namespace instead of failing against one stuck in Terminating.
    kubectl(`delete namespace ${NS} --ignore-not-found --wait=true`);
  });

  test("pod create and delete update /pods without a reload", async ({
    page,
  }) => {
    // A previous attempt (CI retry) may have left the pod behind.
    kubectl(`delete pod ${POD} -n ${NS} --ignore-not-found --wait=true`);

    await page.goto("/pods");
    // Selecting the namespace triggers one filtered list fetch; wait for it
    // so the tracker below only sees requests made after the page settles.
    await Promise.all([
      page.waitForResponse((response) => {
        const url = new URL(response.url());
        return (
          url.pathname.endsWith("/api/pods") &&
          url.searchParams.get("namespace") === NS
        );
      }),
      page.getByRole("combobox").selectOption(NS),
    ]);
    const link = page.getByRole("link", { name: POD, exact: true });
    await expect(link).toHaveCount(0);
    const listRequests = trackListRequests(page, "/api/pods");

    try {
      kubectl("apply -f -", podManifest);
      await expect(link).toBeVisible();

      // The status flips Pending -> Running via "modified" watch events.
      const row = page.locator("tr", { has: link });
      await expect(row.getByText("Running", { exact: true })).toBeVisible();

      kubectl(`delete pod ${POD} -n ${NS} --wait=false`);
      await expect(link).toHaveCount(0);

      // Every transition above must have arrived over the watch stream, not
      // through the error-fallback re-list.
      expect(listRequests).toEqual([]);
    } finally {
      kubectl(`delete pod ${POD} -n ${NS} --ignore-not-found --wait=false`);
    }
  });

  test("namespace create and delete update /namespaces without a reload", async ({
    page,
  }) => {
    const liveNs = "e2e-live-added";
    kubectl(`delete namespace ${liveNs} --ignore-not-found --wait=true`);

    // The initial list fetch is issued only after hydration, so it must be
    // awaited explicitly before the tracker attaches.
    const initialList = page.waitForResponse((response) =>
      new URL(response.url()).pathname.endsWith("/api/namespaces"),
    );
    await page.goto("/namespaces");
    await initialList;
    // Wait for the page to render before asserting absence, so the check
    // does not pass trivially against the loading spinner.
    await expect(
      page.getByRole("heading", { name: "Namespaces", exact: true }),
    ).toBeVisible();
    const card = page.getByRole("heading", { name: liveNs, exact: true });
    await expect(card).toHaveCount(0);
    const listRequests = trackListRequests(page, "/api/namespaces");

    try {
      kubectl(`create namespace ${liveNs}`);
      await expect(card).toBeVisible();

      kubectl(`delete namespace ${liveNs} --wait=false`);
      // Namespace finalization takes a few seconds beyond the delete call.
      await expect(card).toHaveCount(0, { timeout: 30_000 });

      // Card add/remove must have come from watch events, not a re-list.
      expect(listRequests).toEqual([]);
    } finally {
      kubectl(`delete namespace ${liveNs} --ignore-not-found --wait=false`);
    }
  });
});
