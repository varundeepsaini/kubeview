import { test, expect } from "@playwright/test";

test.describe("secrets page", () => {
  test("sidebar navigates to Secrets", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Secrets", exact: true }).click();
    await expect(page).toHaveURL(/\/secrets$/);
    await expect(
      page.getByRole("heading", { name: "Secrets", exact: true }),
    ).toBeVisible();
  });

  test("shows the seeded secret with type, key names and data lengths", async ({
    page,
  }) => {
    await page.goto("/secrets");
    await page.getByRole("combobox").selectOption("e2e-demo");

    const row = page.locator("tr", {
      has: page.getByText("e2e-secret", { exact: true }),
    });
    await expect(row).toBeVisible();
    await expect(row.getByText("Opaque", { exact: true })).toBeVisible();
    await expect(row.getByText("username (8 bytes)")).toBeVisible();
    await expect(row.getByText("password (15 bytes)")).toBeVisible();
  });

  test("never exposes secret values or a reveal control", async ({ page }) => {
    await page.goto("/secrets");
    await page.getByRole("combobox").selectOption("e2e-demo");
    await expect(page.getByText("e2e-secret", { exact: true })).toBeVisible();

    // The plaintext fixture values must never reach the page.
    await expect(page.getByText("e2e-user")).toHaveCount(0);
    await expect(page.getByText("sup3r-s3cret-pw")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /reveal|show|decode/i }),
    ).toHaveCount(0);
  });
});
