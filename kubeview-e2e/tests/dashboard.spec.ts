import { test, expect } from "@playwright/test";

test.describe("dashboard", () => {
  test("renders the heading and cluster summary with a version", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: "Dashboard", exact: true }),
    ).toBeVisible();
    // Cluster line: "Cluster: <name> | Version: <v> | Platform: <p>".
    // Assert an actual version value renders, not just the static label.
    await expect(page.getByText(/Version: v?\d/)).toBeVisible();
  });

  test("shows the resource stat cards with counts", async ({ page }) => {
    await page.goto("/");
    // Scope to the bento grid so we assert the stat CARDS, not the sidebar
    // nav links (which carry the same label text).
    const grid = page.locator(".grid-cols-4").first();
    await expect(grid).toBeVisible();

    for (const label of ["Pods", "Deployments", "Namespaces", "Nodes"]) {
      await expect(grid.getByText(label, { exact: true })).toBeVisible();
    }
    // The Pods/Deployments/Nodes tiles each render a "running/total" count.
    await expect(grid.getByText(/^\d+\/\d+$/).first()).toBeVisible();
  });
});
