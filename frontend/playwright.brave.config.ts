import { defineConfig } from "@playwright/test";

import baseConfig from "./playwright.config";

export default defineConfig({
  ...baseConfig,
  use: {
    ...baseConfig.use,
    launchOptions: {
      executablePath: "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
    }
  }
});
