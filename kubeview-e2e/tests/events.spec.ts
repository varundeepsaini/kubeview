import { test, expect } from "@playwright/test";

test.describe("events page", () => {
  test("shows events for the seeded namespace", async ({ page }) => {
    await page.goto("/events");
    await page.getByRole("combobox").selectOption("e2e-demo");

    // kubelet emits lifecycle events (Scheduled/Pulled/Created/Started) for
    // the fixture pods; the object column references them by name.
    await expect(page.getByText("e2e-logger").first()).toBeVisible();
  });

  test("search narrows events", async ({ page }) => {
    await page.goto("/events");
    await page.getByRole("combobox").selectOption("e2e-demo");
    await expect(page.getByText("e2e-logger").first()).toBeVisible();

    await page.getByPlaceholder(/Search/).fill("e2e-multi");
    await expect(page.getByText("e2e-logger")).toHaveCount(0);
    await expect(page.getByText("e2e-multi").first()).toBeVisible();
  });
});
