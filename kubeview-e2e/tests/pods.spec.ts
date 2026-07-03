import { test, expect } from "@playwright/test";

test.describe("pods page", () => {
  test("namespace filter narrows to the seeded pods", async ({ page }) => {
    await page.goto("/pods");

    // Filter to the fixtures namespace so system pods don't crowd the table.
    await page.getByRole("combobox").selectOption("e2e-demo");

    await expect(
      page.getByRole("link", { name: "e2e-logger", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "e2e-multi", exact: true }),
    ).toBeVisible();
  });

  test("search filters the pod list", async ({ page }) => {
    await page.goto("/pods");
    await page.getByRole("combobox").selectOption("e2e-demo");
    await expect(
      page.getByRole("link", { name: "e2e-logger", exact: true }),
    ).toBeVisible();

    await page.getByPlaceholder("Search pods...").fill("logger");

    await expect(
      page.getByRole("link", { name: "e2e-logger", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "e2e-multi", exact: true }),
    ).toHaveCount(0);
  });
});
