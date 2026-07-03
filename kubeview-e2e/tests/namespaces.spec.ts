import { test, expect } from "@playwright/test";

test("lists the seeded e2e-demo namespace", async ({ page }) => {
  await page.goto("/namespaces");
  await expect(page.getByText("e2e-demo", { exact: true })).toBeVisible();
});
