import { test } from "@playwright/test";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import path from "node:path";
import fs from "node:fs/promises";

const run = promisify(execFile);
const scripts = [
  ["UV05", "uv05-workspace-e2e.cjs"],
  ["UV06", "uv06-requirement-review-e2e.cjs"],
  ["UV07", "uv07-testcase-versioning-e2e.cjs"],
  ["UV08", "uv08-proposal-ui-e2e.cjs"],
];
for (const [phase, script] of scripts) {
  test(`${phase} retained real-backend journey`, async ({}, info) => {
    test.skip(
      !process.env.UV09_STACK_READY,
      "Use npm run test:e2e with an isolated migrated database.",
    );
    const artifacts = info.outputPath("evidence");
    await fs.mkdir(artifacts, { recursive: true });
    try {
      const result = await run(
        process.execPath,
        [path.resolve("../scripts", script)],
        {
          timeout: 220_000,
          maxBuffer: 4 * 1024 * 1024,
          env: {
            ...process.env,
            PLAYWRIGHT_MODULE: require.resolve("playwright"),
            [`${phase}_BASE_URL`]: process.env.UV09_REVIEWER_URL,
            [`${phase}_ARTIFACT_DIR`]: artifacts,
            UV08_VIEWER_URL: process.env.UV09_VIEWER_URL,
          },
        },
      );
      await info.attach("journey-output", {
        body: result.stdout + result.stderr,
        contentType: "text/plain",
      });
    } catch (error) {
      const result = error as Error & { stdout?: string; stderr?: string };
      await info.attach("journey-output", {
        body: `${result.stdout || ""}\n${result.stderr || ""}`,
        contentType: "text/plain",
      });
      throw error;
    } finally {
      for (const file of await fs.readdir(artifacts)) {
        if (file.endsWith(".png"))
          await info.attach(file, {
            path: path.join(artifacts, file),
            contentType: "image/png",
          });
      }
    }
  });
}
