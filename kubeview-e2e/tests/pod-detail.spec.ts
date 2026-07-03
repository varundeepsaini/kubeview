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

  test("overview shows the container count, conditions, and labels", async ({
    page,
  }) => {
    await page.goto("/pods/e2e-demo/e2e-logger");
    await expect(page.getByText("Containers (1)")).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Conditions", exact: true }),
    ).toBeVisible();
    // The fixture's distinctive label value renders in the Labels section.
    await expect(page.getByText("e2e-fixture-label")).toBeVisible();
  });

  test("logs tab streams the container's output", async ({ page }) => {
    await page.goto("/pods/e2e-demo/e2e-logger");
    await page.getByRole("button", { name: "Logs" }).click();

    // The fixture pod echoes this marker every 2s.
    await expect(
      page.getByText("E2E_LOG_LINE_MARKER").first(),
    ).toBeVisible();
  });

  test("can switch back from Logs to Overview", async ({ page }) => {
    await page.goto("/pods/e2e-demo/e2e-logger");
    await page.getByRole("button", { name: "Logs" }).click();
    await expect(
      page.getByText("E2E_LOG_LINE_MARKER").first(),
    ).toBeVisible();

    await page.getByRole("button", { name: "Overview" }).click();
    await expect(page.getByText("Containers (1)")).toBeVisible();
  });

  test("shows an error for a pod that does not exist", async ({ page }) => {
    await page.goto("/pods/e2e-demo/no-such-pod");
    await expect(page.getByText("Pod not found")).toBeVisible();
  });
});
