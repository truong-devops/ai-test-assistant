import { test, expect } from "@playwright/test";
import type { DocumentWorkflow, DocumentWorkflowJob } from "../lib/types";

for (const mode of ["reject400", "timeout", "invalid_enum"]) {
  test(`U04 ${mode}: persisted failure, bounded retry and recovery`, async ({
    page,
    request,
  }, info) => {
    test.skip(
      process.env.UV09_PROVIDER_FIXTURE !== "yes" ||
        !process.env.UV09_STACK_READY,
      "Use UV09_PROVIDER_FIXTURE=yes npm run test:e2e on an isolated database.",
    );
    const control = async (mode: string, reset = false) => {
      const response = await request.post("http://127.0.0.1:8193/control", {
        headers: { Authorization: `Bearer ${process.env.UV09_TEST_TOKEN}` },
        data: { mode, reset },
      });
      expect(response.ok()).toBeTruthy();
    };
    const stats = async () => {
      const response = await request.get("http://127.0.0.1:8193/control", {
        headers: { Authorization: `Bearer ${process.env.UV09_TEST_TOKEN}` },
      });
      expect(response.ok()).toBeTruthy();
      return response.json() as Promise<{ calls: { mode: string }[] }>;
    };
    await control(mode, true);
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto("/documents");
    await page
      .getByLabel("Tên bộ tài liệu", { exact: true })
      .fill(`UV09 ${mode} ${Date.now()}`);
    await page.getByRole("button", { name: "Tạo và mở bộ tài liệu" }).click();
    await page.waitForURL(/\/documents\/\d+\?step=documents/);
    const setID = Number(new URL(page.url()).pathname.split("/").at(-1));
    const base = `/api/backend/api/document-sets/${setID}`;
    const workflow = async () => {
      const response = await page.request.get(`${base}/workflow`);
      expect(response.ok()).toBeTruthy();
      return response.json() as Promise<DocumentWorkflow>;
    };
    await page.locator("input[type=file]").setInputFiles({
      name: "fault.md",
      mimeType: "text/markdown",
      buffer: Buffer.from(
        "The system must create an order when a customer confirms checkout.\n",
      ),
    });
    await page
      .getByRole("button", { name: "Tải tài liệu lên", exact: true })
      .click();
    const row = page.getByRole("row").filter({ hasText: "fault.md" });
    await expect(row.getByText("Đã đọc", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await row.getByRole("checkbox", { name: "Chọn duyệt fault v1" }).check();
    await page.getByLabel("Tên hiển thị người duyệt").fill("UV09 QA");
    await page
      .getByRole("button", { name: "Duyệt nguồn & trích xuất yêu cầu bằng AI" })
      .click();
    // Alert must appear via normal polling; do not refresh to make failure visible.
    await expect(
      page.getByRole("alert").filter({ hasText: "Xử lý nguồn chưa hoàn tất" }),
    ).toBeVisible({ timeout: 45_000 });
    let failed: DocumentWorkflowJob | undefined;
    await expect
      .poll(
        async () => {
          failed = (await workflow()).recent_jobs.find(
            (job) => job.operation === "EXTRACT_REQUIREMENTS",
          );
          return failed?.status;
        },
        { timeout: 30_000 },
      )
      .toBe("FAILED");
    const before = failed!;
    expect(before.attempt_count).toBe(3);
    expect(before.retryable).toBe(true);
    expect(before.error_code).toBe("REQUIREMENT_EXTRACTION_FAILED");
    expect(before.failed_units).toBeGreaterThan(0);
    expect(before.completed_units).toBe(0);
    expect(before.error_message).toMatch(
      mode === "reject400"
        ? /400.*UV09 fixture rejected/
        : mode === "invalid_enum"
          ? /invalid status.*APPROVED/
          : /deadline|timeout/i,
    );
    expect((await stats()).calls.map((call) => call.mode)).toEqual([
      mode,
      mode,
      mode,
    ]);
    expect(
      (await (await page.request.get(`${base}/requirements`)).json())
        .requirements,
    ).toEqual([]);

    await page.reload();
    await page.getByText("Chi tiết xử lý", { exact: true }).click();
    const jobPanel = page
      .locator(".workflow-job-detail")
      .filter({ hasText: `Job #${before.id} ·` });
    await jobPanel.getByText("Chi tiết lỗi xử lý", { exact: true }).click();
    await expect(jobPanel.locator("pre")).toContainText(
      before.error_message ?? "",
    );
    await expect(
      jobPanel.getByRole("button", { name: "Thử lại bước lỗi" }),
    ).toBeEnabled();
    const afterReload = (await workflow()).recent_jobs.find(
      (job) => job.id === before.id,
    )!;
    expect(afterReload.attempt_count).toBe(before.attempt_count);
    expect(afterReload.completed_units).toBe(before.completed_units);
    expect((await stats()).calls).toHaveLength(3);
    await page.screenshot({
      path: info.outputPath(`${mode}-failed.png`),
      fullPage: true,
    });
    await info.attach("persisted-failure-screen", {
      path: info.outputPath(`${mode}-failed.png`),
      contentType: "image/png",
    });
    await info.attach("failed-job", {
      body: JSON.stringify(before, null, 2),
      contentType: "application/json",
    });

    await control("valid");
    await jobPanel.getByRole("button", { name: "Thử lại bước lỗi" }).click();
    await expect
      .poll(
        async () =>
          (await workflow()).recent_jobs.find((job) => job.id === before.id)
            ?.status,
        { timeout: 30_000 },
      )
      .toBe("SUCCEEDED");
    await expect
      .poll(
        async () =>
          (await workflow()).source_intents.find(
            (intent) => intent.extraction_job_id === before.id,
          )?.status,
        { timeout: 30_000 },
      )
      .toBe("SUCCEEDED");
    const recovered = (await workflow()).recent_jobs.filter(
      (job) => job.operation === "EXTRACT_REQUIREMENTS",
    );
    expect(recovered).toHaveLength(1);
    expect(recovered[0].id).toBe(before.id);
    const requirements = (
      await (await page.request.get(`${base}/requirements`)).json()
    ).requirements;
    expect(requirements).toHaveLength(1);
    expect(requirements[0].status).toBe("DRAFT");
    expect((await stats()).calls.map((call) => call.mode)).toEqual([
      mode,
      mode,
      mode,
      "valid",
    ]);
    expect(errors).toEqual([]);
    // Exercise the distinct proposal-generation checkpoint/error path too.
    const approved = await page.request.post(
      `${base}/requirement-review/bulk-review`,
      {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: {
          items: [
            {
              id: requirements[0].id,
              expected_hash: requirements[0].review_hash,
            },
          ],
          decision: "APPROVED",
          reviewer_name: "UV09 QA",
          comment: "Grounded fixture",
        },
      },
    );
    expect(approved.ok()).toBeTruthy();
    expect((await approved.json()).results[0].status).toBe("APPLIED");
    await control(mode);
    await page.goto(`/documents/${setID}?step=test-cases`);
    await page
      .getByLabel("Phạm vi sinh đề xuất", { exact: true })
      .selectOption("all");
    await page
      .getByRole("checkbox", { name: /Tôi xác nhận sinh đề xuất/ })
      .check();
    await page
      .getByRole("button", { name: "Sinh đề xuất để review", exact: true })
      .click();
    let generation: DocumentWorkflowJob | undefined;
    await expect
      .poll(
        async () => {
          generation = (await workflow()).recent_jobs.find(
            (job) => job.operation === "GENERATE_TESTCASES",
          );
          return generation?.status;
        },
        { timeout: 30_000 },
      )
      .toBe("PARTIAL_FAILED");
    const generationID = generation!.id;
    expect(generation!.completed_units).toBe(1); // retire checkpoint, no provider call
    expect(generation!.failed_units).toBe(1);
    // Generation retries are explicit per failed unit, not a provider-call loop.
    expect((await stats()).calls.map((call) => call.mode)).toEqual([
      mode,
      mode,
      mode,
      "valid",
      mode,
    ]);
    // Select the top action, not the historical job details.
    const retry = page
      .getByRole("button", { name: "Thử lại bước lỗi", exact: true })
      .first();
    await expect(retry).toBeVisible({ timeout: 15_000 });
    await control("hold");
    await retry.click();
    await expect
      .poll(async () => (await stats()).calls.at(-1)?.mode, {
        timeout: 10_000,
        intervals: [100],
      })
      .toBe("hold");
    // Reload in the middle of generate: same persisted operation, no POST replay.
    await page.reload();
    expect(
      (await workflow()).recent_jobs.find((job) => job.id === generationID)
        ?.status,
    ).toBe("RUNNING");
    await control("valid");
    await expect
      .poll(
        async () =>
          (await workflow()).recent_jobs.find((job) => job.id === generationID)
            ?.status,
        { timeout: 30_000 },
      )
      .toBe("SUCCEEDED");
    expect(
      (await workflow()).recent_jobs.filter(
        (job) => job.operation === "GENERATE_TESTCASES",
      ),
    ).toHaveLength(1);
    expect((await stats()).calls.map((call) => call.mode)).toEqual([
      mode,
      mode,
      mode,
      "valid",
      mode,
      "hold",
    ]);
    expect(
      (await (await page.request.get(`${base}/test-cases`)).json()).test_cases,
    ).toEqual([]);
    const proposals = (
      await (
        await page.request.get(
          `${base}/test-case-proposals?job_id=${generationID}`,
        )
      ).json()
    ).proposals;
    expect(proposals).toHaveLength(1);
    expect(proposals[0].status).toBe("PENDING");
    const budget = await (await page.request.get(`${base}/ai-budget`)).json();
    expect(budget.unreconciled_reservations).toBe(
      mode === "invalid_enum" ? 0 : 4,
    );
    expect(budget.used_tokens).toBe(mode === "invalid_enum" ? 240 : 80);
    if (mode !== "invalid_enum") {
      expect(budget.reserved_tokens).toBeGreaterThan(0);
      await page.reload();
      await page
        .getByText("Retention, AI budget, and deletion policy", { exact: true })
        .click();
      await expect(
        page
          .getByRole("alert")
          .filter({ hasText: "reservation quá hạn chưa đối soát" }),
      ).toBeVisible();
    }
    await info.attach("budget-reconciliation-state", {
      body: JSON.stringify(budget, null, 2),
      contentType: "application/json",
    });
    expect(errors).toEqual([]);
    await info.attach("generation-recovered", {
      body: JSON.stringify(
        (await workflow()).recent_jobs.find((job) => job.id === generationID),
        null,
        2,
      ),
      contentType: "application/json",
    });
    await info.attach("recovered-job", {
      body: JSON.stringify(recovered[0], null, 2),
      contentType: "application/json",
    });
  });
}
