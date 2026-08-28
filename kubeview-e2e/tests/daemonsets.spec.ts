import { test, expect } from "@playwright/test";

test.describe("daemonsets page", () => {
  test("sidebar navigates to DaemonSets", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "DaemonSets", exact: true }).click();
    await expect(page).toHaveURL(/\/daemonsets$/);
    await expect(
      page.getByRole("heading", { name: "DaemonSets", exact: true }),
    ).toBeVisible();
  });

  test("shows the seeded daemonset with scheduling counts", async ({ page }) => {
    await page.goto("/daemonsets");
    await page.getByRole("combobox", { name: "Filter by namespace" }).selectOption("e2e-demo");

    const row = page.locator("tr", {
      has: page.getByText("e2e-agent", { exact: true }),
    });
    await expect(row).toBeVisible();
    // Single-node cluster with the rollout complete: desired, current, ready,
    // up-to-date and available are all 1.
    await expect(row.getByText("1", { exact: true })).toHaveCount(5);
  });

  test("namespace filter hides system daemonsets", async ({ page }) => {
    await page.goto("/daemonsets");
    // Unfiltered, kind's kube-system daemonsets are listed.
    await expect(page.getByText("kube-proxy", { exact: true })).toBeVisible();

    await page.getByRole("combobox", { name: "Filter by namespace" }).selectOption("e2e-demo");
    await expect(page.getByText("e2e-agent", { exact: true })).toBeVisible();
    await expect(page.getByText("kube-proxy", { exact: true })).toHaveCount(0);
  });
});
