import { test, expect } from "@playwright/test";

test.describe("dashboard", () => {
  test("renders the heading and cluster summary", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: "Dashboard", exact: true }),
    ).toBeVisible();
    // Cluster line: "Cluster: <name> | Version: <v> | Platform: <p>".
    await expect(page.getByText(/Version:/)).toBeVisible();
  });

  test("shows the resource stat cards", async ({ page }) => {
    await page.goto("/");
    for (const label of ["Pods", "Deployments", "Namespaces", "Nodes"]) {
      await expect(page.getByText(label, { exact: true }).first()).toBeVisible();
    }
    // Pods tile shows a running/total count like "N/M".
    await expect(page.getByText("Running", { exact: true }).first()).toBeVisible();
  });
});
