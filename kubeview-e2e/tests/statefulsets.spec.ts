import { test, expect } from "@playwright/test";

test.describe("statefulsets page", () => {
  test("sidebar navigates to StatefulSets", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "StatefulSets", exact: true }).click();
    await expect(page).toHaveURL(/\/statefulsets$/);
    await expect(
      page.getByRole("heading", { name: "StatefulSets", exact: true }),
    ).toBeVisible();
  });

  test("shows the seeded statefulset with ready/desired counts", async ({
    page,
  }) => {
    await page.goto("/statefulsets");
    await page.getByRole("combobox").selectOption("e2e-demo");

    const row = page.locator("tr", {
      has: page.getByText("e2e-db", { exact: true }),
    });
    await expect(row).toBeVisible();
    // Ready column uses ready/desired form; CI waits for the rollout, so the
    // single replica is up.
    await expect(row.getByText("1/1", { exact: true })).toBeVisible();
    await expect(row.getByText("e2e-db-hl", { exact: true })).toBeVisible();
    await expect(row.getByText("RollingUpdate", { exact: true })).toBeVisible();
  });
});
