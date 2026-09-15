import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

export default defineConfig({
  ...base,
  testMatch: "mobile-responsive.spec.ts",
  timeout: 300_000,
  retries: 0,
  workers: 1,
  use: { ...base.use, viewport: { width: 390, height: 844 }, hasTouch: true, actionTimeout: 20_000 },
  projects: [
    { name: "mobile-chromium", use: { browserName: "chromium" } },
    { name: "mobile-webkit", use: { browserName: "webkit" } }
  ]
});
