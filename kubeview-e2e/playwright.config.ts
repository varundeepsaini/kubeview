import { defineConfig, devices } from "@playwright/test";

// The UI polls every 5s, so assertions need to tolerate a poll cycle. A
// generous expect timeout lets Playwright's auto-retry ride out one refresh.
const EXPECT_TIMEOUT = 15_000;

const FRONTEND_URL = "http://localhost:5500";
const BACKEND_URL = "http://localhost:5501";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  timeout: 60_000,
  expect: { timeout: EXPECT_TIMEOUT },
  use: {
    baseURL: FRONTEND_URL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
  // Bring up the real services. The kind cluster + fixtures must already be
  // applied (CI does this before invoking Playwright; locally, apply
  // fixtures.yaml against your cluster first). The frontend must be built
  // (`next build`) beforehand — its API base is inlined at build time and
  // defaults to http://localhost:5501/api.
  webServer: [
    {
      command:
        "cd ../kubeview-backend && CORS_ORIGIN=http://localhost:5500 PORT=5501 go run .",
      url: `${BACKEND_URL}/api/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      // Run the standalone server — the exact artifact the Docker image ships
      // (next.config.ts sets output: "standalone"). The build emits the
      // server under .next/standalone; its static assets and public/ must sit
      // beside it, mirroring the Dockerfile's COPY steps.
      command:
        "cd ../kubeview-frontend && cp -r .next/static .next/standalone/.next/ && cp -r public .next/standalone/ && PORT=5500 HOSTNAME=127.0.0.1 node .next/standalone/server.js",
      url: FRONTEND_URL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      stdout: "pipe",
      stderr: "pipe",
    },
  ],
});
