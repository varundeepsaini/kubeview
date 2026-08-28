import { execSync } from "node:child_process";
import { test, expect, type Page } from "@playwright/test";

// Exercises the cluster flight recorder end to end: a pod that no longer
// exists must reappear when the timeline scrubber is dragged to a moment
// before its deletion, rendered from /api/history/state with no live list
// traffic — and disappear again on returning to live.

const NS = "e2e-history";
const POD = "e2e-history-pod";

// Committing within 10s of the range's end snaps back to live (see
// TimelineBar), so the pre-delete moment must age past that window before
// the scrub.
const LIVE_SNAP_MS = 10_000;

const kubectl = (args: string, input?: string) =>
  execSync(`kubectl ${args}`, { encoding: "utf8", input });

const trackListRequests = (page: Page, apiPath: string): string[] => {
  const requests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.endsWith(apiPath)) {
      requests.push(request.url());
    }
  });
  return requests;
};

const namespaceManifest = `apiVersion: v1\nkind: Namespace\nmetadata:\n  name: ${NS}\n`;

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

// Drives the scrubber to a moment: React range inputs need the native value
// setter (React's value tracking swallows plain .value writes), an input
// event for onChange, and a pointerup to commit.
async function scrubTo(page: Page, momentMs: number) {
  const slider = page.getByRole("slider", { name: "Timeline scrubber" });
  await expect(slider).toBeVisible();
  await slider.evaluate((node, value) => {
    const input = node as HTMLInputElement;
    const setter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    )!.set!;
    setter.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new PointerEvent("pointerup", { bubbles: true }));
  }, String(momentMs));
}

// The namespaced setup lives inside this describe alone: fullyParallel runs
// each test in its own worker, and workers repeat describe-level hooks — a
// second test sharing the namespace would race its deletion.
test.describe("cluster flight recorder", () => {
  test.beforeAll(() => {
    // A leftover namespace from a crashed run may still be terminating;
    // wait out the deletion before recreating it.
    kubectl(`delete namespace ${NS} --ignore-not-found --wait=true`);
    kubectl("apply -f -", namespaceManifest);
  });

  test.afterAll(() => {
    kubectl(`delete namespace ${NS} --ignore-not-found --wait=true`);
  });

  test("scrubbing back shows a deleted pod; returning to live hides it", async ({
    page,
  }) => {
    test.slow(); // deliberately waits out the live-snap window

    kubectl(`delete pod ${POD} -n ${NS} --ignore-not-found --wait=true`);
    kubectl("apply -f -", podManifest);
    try {
      kubectl(`wait --for=condition=Ready pod/${POD} -n ${NS} --timeout=60s`);
      // Give the recorder a beat to persist the Running state, then stamp
      // the moment the pod verifiably existed.
      await new Promise((resolve) => setTimeout(resolve, 3_000));
      const beforeDelete = Date.now();

      kubectl(`delete pod ${POD} -n ${NS} --wait=true`);

      // Age the stamped moment out of the scrubber's snap-to-live window.
      await new Promise((resolve) =>
        setTimeout(resolve, LIVE_SNAP_MS + 2_000),
      );

      // The initial list fetch is issued only after hydration, so await it
      // explicitly before asserting on the rendered table.
      const initialList = page.waitForResponse((response) =>
        new URL(response.url()).pathname.endsWith("/api/pods"),
      );
      await page.goto("/pods");
      await initialList;
      const link = page.getByRole("link", { name: POD, exact: true });
      await expect(
        page.getByRole("heading", { name: "Pods", exact: true }),
      ).toBeVisible();
      await expect(link).toHaveCount(0);

      await scrubTo(page, beforeDelete);
      await expect(page.getByText(/Viewing past/)).toBeVisible();
      await expect(link).toBeVisible();

      // Once past mode is engaged the remount has closed every live watch
      // and cleared its retry timers, so no live list may fire from here on.
      // (While still live, a starved SSE connection legitimately re-lists —
      // the whole suite shares one browser connection pool, so asserting an
      // empty window before the commit is flaky under parallel load.)
      const liveListRequests = trackListRequests(page, "/api/pods");
      await page.waitForTimeout(2_000);
      expect(liveListRequests).toEqual([]);

      // Back to live: the pod is gone again.
      await page.getByRole("button", { name: /LIVE/ }).click();
      await expect(page.getByText(/Viewing past/)).toHaveCount(0);
      await expect(link).toHaveCount(0);
    } finally {
      kubectl(`delete pod ${POD} -n ${NS} --ignore-not-found --wait=false`);
    }
  });

});

// Needs no cluster fixtures, so it lives outside the namespaced describe.
test("timeline page compares two moments", async ({ page }) => {
  await page.goto("/timeline");
  await expect(
    page.getByRole("heading", { name: "Timeline", exact: true }),
  ).toBeVisible();

  // The backend records from startup, so a preset comparison always has a
  // valid window; assert the diff renders (summary counts + tables). The
  // "N added" phrasing pins the summary line — bare "added" would also
  // match every change badge in the table below.
  await page.getByRole("button", { name: "Last hour" }).click();
  await expect(page.getByText(/\d+ added/)).toBeVisible();
  await expect(page.getByText(/\d+ removed/)).toBeVisible();
  await expect(page.getByText(/\d+ modified/)).toBeVisible();
  await expect(page.getByText(/Events in this window/)).toBeVisible();
});
