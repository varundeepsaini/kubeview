import { test, expect } from "@playwright/test";

test.describe("ingresses page", () => {
  test("sidebar navigates to Ingresses", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Ingresses", exact: true }).click();
    await expect(page).toHaveURL(/\/ingresses$/);
    await expect(
      page.getByRole("heading", { name: "Ingresses", exact: true }),
    ).toBeVisible();
  });

  test("shows the seeded ingress rule with host, path and backend", async ({
    page,
  }) => {
    await page.goto("/ingresses");
    await page.getByRole("combobox", { name: "Filter by namespace" }).selectOption("e2e-demo");

    const row = page.locator("tr", {
      has: page.getByText("e2e-ing-paths", { exact: true }),
    });
    await expect(row).toBeVisible();
    await expect(row.getByText("e2e.example.com/ -> e2e-svc:80")).toBeVisible();
  });

  test("host-only rule (no http section) renders without breaking the page", async ({
    page,
  }) => {
    await page.goto("/ingresses");
    await page.getByRole("combobox", { name: "Filter by namespace" }).selectOption("e2e-demo");

    // Regression guard: a rule without an http section used to nil-panic the
    // backend. The ingress must still list, with an empty hosts/paths cell.
    const row = page.locator("tr", {
      has: page.getByText("e2e-ing-hostonly", { exact: true }),
    });
    await expect(row).toBeVisible();
    await expect(row.getByText("<none>").first()).toBeVisible();
    // And the rest of the table renders alongside it (the endpoint didn't 500).
    await expect(page.getByText("e2e-ing-paths", { exact: true })).toBeVisible();
  });
});
