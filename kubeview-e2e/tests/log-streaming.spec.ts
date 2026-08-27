import { test, expect, type Page } from "@playwright/test";

// The log pane renders one div per line, so counting marker matches counts
// rendered lines. e2e-logger emits E2E_LOG_LINE_MARKER every 2s, which lets
// these tests distinguish a live follow from a one-shot tail.

async function openLogs(page: Page, path: string) {
  await page.goto(path);
  await page.getByRole("button", { name: "Logs" }).click();
}

test.describe("log streaming", () => {
  test("follow mode keeps appending new lines", async ({ page }) => {
    await openLogs(page, "/pods/e2e-demo/e2e-logger");
    const lines = page.getByText("E2E_LOG_LINE_MARKER");
    await expect(lines.first()).toBeVisible();

    const initial = await lines.count();
    await expect
      .poll(() => lines.count(), { timeout: 15_000 })
      .toBeGreaterThan(initial);
  });

  test("pause freezes the pane and resume flushes the buffered lines", async ({
    page,
  }) => {
    await openLogs(page, "/pods/e2e-demo/e2e-logger");
    const lines = page.getByText("E2E_LOG_LINE_MARKER");
    await expect(lines.first()).toBeVisible();

    await page.getByRole("button", { name: "Pause", exact: true }).click();
    const frozen = await lines.count();
    // The marker arrives every 2s, so this window spans at least two new
    // lines; a frozen pane is only provable by letting that time elapse.
    await page.waitForTimeout(5_000);
    await expect(lines).toHaveCount(frozen);

    await page.getByRole("button", { name: "Resume", exact: true }).click();
    // The lines withheld while paused flush on resume, jumping the count
    // past the frozen value.
    await expect
      .poll(() => lines.count(), { timeout: 15_000 })
      .toBeGreaterThan(frozen);
    await expect(
      page.getByRole("button", { name: "Pause", exact: true }),
    ).toBeVisible();
  });

  test("container picker streams the selected container's logs", async ({
    page,
  }) => {
    await openLogs(page, "/pods/e2e-demo/e2e-multi");
    await expect(page.getByText("E2E_MAIN_MARKER").first()).toBeVisible();

    await page.getByRole("combobox").selectOption("sidecar");
    const sidecarLines = page.getByText("E2E_SIDECAR_MARKER");
    await expect(sidecarLines.first()).toBeVisible();
    // Switching restarts the stream, so no main-container output survives.
    await expect(page.getByText("E2E_MAIN_MARKER")).toHaveCount(0);

    // The switched stream follows too, rather than serving a one-shot tail.
    const initial = await sidecarLines.count();
    await expect
      .poll(() => sidecarLines.count(), { timeout: 15_000 })
      .toBeGreaterThan(initial);
  });
});
