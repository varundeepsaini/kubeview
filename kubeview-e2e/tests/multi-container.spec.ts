import { test, expect } from "@playwright/test";

// Regression coverage for the multi-container fixes (issues #4/#6): the logs
// tab must offer a per-container picker and default to a concrete container.
test.describe("multi-container pod", () => {
  test("overview lists both containers", async ({ page }) => {
    await page.goto("/pods/e2e-demo/e2e-multi");
    await expect(
      page.getByRole("heading", { name: "e2e-multi", exact: true }),
    ).toBeVisible();
    await expect(page.getByText("main", { exact: true })).toBeVisible();
    await expect(page.getByText("sidecar", { exact: true })).toBeVisible();
  });

  test("logs tab exposes a container picker with both containers", async ({
    page,
  }) => {
    await page.goto("/pods/e2e-demo/e2e-multi");
    await page.getByRole("button", { name: "Logs" }).click();

    // The picker only renders for multi-container pods.
    const picker = page.getByRole("combobox");
    await expect(picker).toBeVisible();
    await expect(picker.getByRole("option")).toHaveCount(2);

    // Selecting the sidecar must not error the log pane.
    await picker.selectOption("sidecar");
    await expect(page.getByText(/HTTP-Code: 400/)).toHaveCount(0);
  });
});
