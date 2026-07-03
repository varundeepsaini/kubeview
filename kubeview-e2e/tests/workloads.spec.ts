import { test, expect } from "@playwright/test";

test("deployments page shows the seeded deployment", async ({ page }) => {
  await page.goto("/deployments");
  await page.getByRole("combobox").selectOption("e2e-demo");
  await expect(page.getByText("e2e-web", { exact: true })).toBeVisible();
});

test("services page shows the seeded service", async ({ page }) => {
  await page.goto("/services");
  await page.getByRole("combobox").selectOption("e2e-demo");
  await expect(page.getByText("e2e-svc", { exact: true })).toBeVisible();
});

test("nodes page lists at least the cluster node", async ({ page }) => {
  await page.goto("/nodes");
  // kind names its node "<cluster>-control-plane"; assert the row exists.
  await expect(page.getByText(/control-plane/).first()).toBeVisible();
});
