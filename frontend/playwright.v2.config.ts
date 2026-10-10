import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "v2-refunds.spec.ts",
  outputDir: process.env.V2_E2E_OUTPUT_DIR || "test-results/v2",
  workers: 1,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  use: { baseURL: process.env.E2E_BASE_URL, trace: "off", screenshot: "off" },
  projects: [
    { name: "desktop", use: { viewport: { width: 1280, height: 900 } } },
    { name: "tablet", use: { viewport: { width: 820, height: 1180 } } },
    { name: "mobile", use: { viewport: { width: 390, height: 844 } } }
  ]
});
