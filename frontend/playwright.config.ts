import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  // A dev server compiles each route on first request, so a test that walks
  // many pages spends most of its budget waiting on webpack, not on the app.
  timeout: 180_000,
  expect: { timeout: 15_000 },
  // One retry absorbs first-compile stalls without hiding a repeatable failure.
  retries: 1,
  use: {
    baseURL: process.env.E2E_BASE_URL || "http://localhost:3000",
    trace: "retain-on-failure"
  },
  webServer: process.env.E2E_SKIP_WEBSERVER
    ? undefined
    : {
        command: "npm run dev",
        port: 3000,
        reuseExistingServer: true
      }
});
