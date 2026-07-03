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

  test("shows Running status and ready count for a seeded pod", async ({
    page,
  }) => {
    await page.goto("/pods");
    await page.getByRole("combobox").selectOption("e2e-demo");

    const row = page.locator("tr", {
      has: page.getByRole("link", { name: "e2e-logger", exact: true }),
    });
    await expect(row.getByText("Running", { exact: true })).toBeVisible();
    await expect(row.getByText("1/1", { exact: true })).toBeVisible();
  });

  test("a non-matching search yields an empty table", async ({ page }) => {
    await page.goto("/pods");
    await page.getByRole("combobox").selectOption("e2e-demo");
    await expect(
      page.getByRole("link", { name: "e2e-logger", exact: true }),
    ).toBeVisible();

    await page.getByPlaceholder("Search pods...").fill("zzz-no-such-pod-zzz");
    await expect(page.getByRole("link", { name: /e2e-/ })).toHaveCount(0);
  });

  test("clicking a pod name navigates to its detail page", async ({ page }) => {
    await page.goto("/pods");
    await page.getByRole("combobox").selectOption("e2e-demo");
    await page.getByRole("link", { name: "e2e-logger", exact: true }).click();

    await expect(page).toHaveURL(/\/pods\/e2e-demo\/e2e-logger$/);
    await expect(
      page.getByRole("heading", { name: "e2e-logger", exact: true }),
    ).toBeVisible();
  });
});
