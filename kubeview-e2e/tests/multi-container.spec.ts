import { test, expect } from "@playwright/test";

// Regression coverage for the multi-container fixes (issues #4/#6): the logs
// tab must offer a per-container picker and default to a concrete container.
test.describe("multi-container pod", () => {
  test("overview lists both containers with their image", async ({ page }) => {
    await page.goto("/pods/e2e-demo/e2e-multi");
    await expect(
      page.getByRole("heading", { name: "e2e-multi", exact: true }),
    ).toBeVisible();
    await expect(page.getByText("Containers (2)")).toBeVisible();
    await expect(page.getByText("main", { exact: true })).toBeVisible();
    await expect(page.getByText("sidecar", { exact: true })).toBeVisible();
    // Both containers run the same fixture image.
    await expect(page.getByText("busybox:1.36")).toHaveCount(2);
  });

  test("logs default to the first container and the picker switches containers", async ({
    page,
  }) => {
    await page.goto("/pods/e2e-demo/e2e-multi");
    await page.getByRole("button", { name: "Logs" }).click();

    // The picker only renders for multi-container pods, with both containers.
    const picker = page.getByRole("combobox");
    await expect(picker).toBeVisible();
    await expect(picker.getByRole("option")).toHaveCount(2);

    // Default (first) container's real output must render — this is the #4/#6
    // regression guard: the pre-fix bug sent no container and got a 400, so
    // no marker would ever appear.
    await expect(page.getByText("E2E_MAIN_MARKER").first()).toBeVisible();
    await expect(page.getByText("E2E_SIDECAR_MARKER")).toHaveCount(0);

    // Switching to the sidecar shows *its* output, not the main container's.
    await picker.selectOption("sidecar");
    await expect(page.getByText("E2E_SIDECAR_MARKER").first()).toBeVisible();
  });
});
