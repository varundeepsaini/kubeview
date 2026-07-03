import { test, expect } from "@playwright/test";

test.describe("pod detail", () => {
  test("overview shows the container and status", async ({ page }) => {
    await page.goto("/pods/e2e-demo/e2e-logger");

    await expect(
      page.getByRole("heading", { name: "e2e-logger", exact: true }),
    ).toBeVisible();
    // The single container is named "logger".
    await expect(page.getByText("logger", { exact: true })).toBeVisible();
  });

  test("logs tab streams the container's output", async ({ page }) => {
    await page.goto("/pods/e2e-demo/e2e-logger");
    await page.getByRole("button", { name: "Logs" }).click();

    // The fixture pod echoes this marker every 2s.
    await expect(
      page.getByText("E2E_LOG_LINE_MARKER").first(),
    ).toBeVisible();
  });
});
