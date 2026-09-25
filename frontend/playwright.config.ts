import { defineConfig } from "@playwright/test";

const faultFixture = process.env.UV09_PROVIDER_FIXTURE === "yes";
const artifacts = faultFixture
  ? "test-results/provider-faults"
  : "test-results";
export default defineConfig({
  testDir: "./e2e",
  timeout: 240_000,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  reporter: [
    ["list"],
    [
      "html",
      {
        open: "never",
        outputFolder: faultFixture
          ? "playwright-report/provider-faults"
          : "playwright-report/journeys",
      },
    ],
    ["junit", { outputFile: `${artifacts}/junit.xml` }],
  ],
  outputDir: `${artifacts}/browser`,
  use: {
    baseURL: process.env.UV09_REVIEWER_URL || "http://127.0.0.1:3190",
    browserName: "chromium",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
