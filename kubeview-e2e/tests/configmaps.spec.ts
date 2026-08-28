import { test, expect } from "@playwright/test";

test.describe("configmaps page", () => {
  test("sidebar navigates to ConfigMaps", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "ConfigMaps", exact: true }).click();
    await expect(page).toHaveURL(/\/configmaps$/);
    await expect(
      page.getByRole("heading", { name: "ConfigMaps", exact: true }),
    ).toBeVisible();
  });

  test("shows the seeded configmap with its keys", async ({ page }) => {
    await page.goto("/configmaps");
    await page.getByRole("combobox", { name: "Filter by namespace" }).selectOption("e2e-demo");

    const row = page.locator("tr", {
      has: page.getByText("e2e-config", { exact: true }),
    });
    await expect(row).toBeVisible();
    // Keys column lists the data keys, sorted alphabetically.
    await expect(row.getByText("greeting, log-level")).toBeVisible();
  });

  test("namespace filter narrows the list to e2e-demo", async ({ page }) => {
    await page.goto("/configmaps");
    // Unfiltered, system configmaps appear (kube-root-ca.crt exists in every
    // namespace, so kube-system rows are guaranteed). Assertions are scoped to
    // table cells because the namespace dropdown also contains "kube-system".
    await expect(
      page.getByRole("cell", { name: "kube-system", exact: true }).first(),
    ).toBeVisible();

    await page.getByRole("combobox", { name: "Filter by namespace" }).selectOption("e2e-demo");
    await expect(page.getByText("e2e-config", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "kube-system", exact: true }),
    ).toHaveCount(0);
  });
});
