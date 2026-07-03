import { test, expect } from "@playwright/test";

// Every sidebar destination loads and shows its heading.
const pages = [
  { link: "Namespaces", heading: "Namespaces", path: "/namespaces" },
  { link: "Pods", heading: "Pods", path: "/pods" },
  { link: "Deployments", heading: "Deployments", path: "/deployments" },
  { link: "Services", heading: "Services", path: "/services" },
  { link: "Events", heading: "Events", path: "/events" },
  { link: "Nodes", heading: "Nodes", path: "/nodes" },
];

test.describe("sidebar navigation", () => {
  for (const p of pages) {
    test(`navigates to ${p.link}`, async ({ page }) => {
      await page.goto("/");
      await page.getByRole("link", { name: p.link, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${p.path}$`));
      await expect(
        page.getByRole("heading", { name: p.heading, exact: true }),
      ).toBeVisible();
    });
  }
});
