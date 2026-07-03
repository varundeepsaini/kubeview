import { test, expect } from "@playwright/test";

test("deployments page shows the seeded deployment", async ({ page }) => {
  await page.goto("/deployments");
  await page.getByRole("combobox").selectOption("e2e-demo");
  await expect(page.getByText("e2e-web", { exact: true })).toBeVisible();
});

test("deployments search narrows the list", async ({ page }) => {
  await page.goto("/deployments");
  await page.getByRole("combobox").selectOption("e2e-demo");
  await expect(page.getByText("e2e-web", { exact: true })).toBeVisible();
  await page.getByPlaceholder(/Search/).fill("nonexistent-deploy");
  await expect(page.getByText("e2e-web", { exact: true })).toHaveCount(0);
});

test("services page shows the seeded service and its type", async ({ page }) => {
  await page.goto("/services");
  await page.getByRole("combobox").selectOption("e2e-demo");

  const row = page.locator("tr", {
    has: page.getByText("e2e-svc", { exact: true }),
  });
  await expect(row).toBeVisible();
  // Only e2e-svc is in the namespace, and it's a ClusterIP.
  await expect(row.getByText("ClusterIP")).toBeVisible();
});

test("nodes page shows the cluster node as Ready with a version", async ({
  page,
}) => {
  await page.goto("/nodes");
  await expect(page.getByText(/control-plane/).first()).toBeVisible();
  // Node card carries a Ready status badge and a Kubernetes version.
  await expect(page.getByText("Ready", { exact: true }).first()).toBeVisible();
  await expect(page.getByText(/^v1\./).first()).toBeVisible();
});

test("namespaces page shows e2e-demo as Active", async ({ page }) => {
  await page.goto("/namespaces");
  const card = page.locator("div", {
    has: page.getByRole("heading", { name: "e2e-demo", exact: true }),
  });
  await expect(card.getByText("Active").first()).toBeVisible();
});

test("namespaces search filters the cards", async ({ page }) => {
  await page.goto("/namespaces");
  await expect(page.getByText("e2e-demo", { exact: true })).toBeVisible();
  await page.getByPlaceholder(/Search/).fill("e2e-demo");
  await expect(page.getByText("e2e-demo", { exact: true })).toBeVisible();
  await expect(page.getByText("kube-system", { exact: true })).toHaveCount(0);
});
