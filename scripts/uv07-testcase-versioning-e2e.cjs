// Creates retained fixtures on an isolated schema-28 API/worker/production UI.
// LLM_PROVIDER=disabled. Never run against production.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1000 },
  });
  page.setDefaultTimeout(30000);
  const base = process.env.UV07_BASE_URL || "http://127.0.0.1:3187";
  const out = process.env.UV07_ARTIFACT_DIR || "/tmp/uv07-evidence";
  fs.mkdirSync(out, { recursive: true });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  let setID;
  const api = async (route, data, headers = {}) => {
    const url = `${base}/api/backend/api/${route}`;
    const r =
      data === undefined
        ? await page.request.get(url)
        : await page.request.post(url, { data, headers });
    assert.ok(r.ok(), `${route}: ${r.status()} ${await r.text()}`);
    return r.json();
  };
  const until = async (fn, predicate) => {
    const end = Date.now() + 90000;
    while (Date.now() < end) {
      const value = await fn();
      if (predicate(value)) return value;
      await new Promise((r) => setTimeout(r, 800));
    }
    throw new Error("Durable state timed out");
  };
  try {
    setID = (await api("document-sets", { name: `UV07 browser ${Date.now()}` }))
      .id;
    const upload = await page.request.post(
      `${base}/api/backend/api/document-sets/${setID}/documents`,
      {
        multipart: {
          new_document: "true",
          file: {
            name: "order.md",
            mimeType: "text/markdown",
            buffer: Buffer.from(
              "# REQ-101\n\nThe system must reject expired coupons and show a validation error.\n",
            ),
          },
        },
      },
    );
    assert.equal(upload.status(), 202);
    const docs = await until(
      () => api(`document-sets/${setID}/documents`),
      (v) => v.documents[0]?.latest_version.parse_status === "PARSED",
    );
    const version = docs.documents[0].latest_version;
    const w = await api(`document-sets/${setID}/workflow`);
    await api(
      `document-sets/${setID}/source-review`,
      {
        command: "APPROVE_AND_EXTRACT",
        expected_source_revision: w.source_revision,
        selected: [
          {
            version_id: version.id,
            sha256: version.sha256,
            approval_status: version.approval_status,
          },
        ],
        excluded_version_ids: [],
        reviewer_name: "Fixture QA",
      },
      { "Idempotency-Key": `uv07-source-${setID}` },
    );
    await until(
      () => api(`document-sets/${setID}/workflow`),
      (v) =>
        v.source_intents.some(
          (i) =>
            i.command === "APPROVE_AND_EXTRACT" && i.status === "SUCCEEDED",
        ),
    );
    const req = (await api(`document-sets/${setID}/requirements`))
      .requirements[0];
    await api(
      `document-sets/${setID}/requirement-review/bulk-review`,
      {
        items: [{ id: req.id, expected_hash: req.review_hash }],
        decision: "APPROVED",
        reviewer_name: "Fixture QA",
      },
      { "Idempotency-Key": `uv07-req-${setID}` },
    );
    await api(
      `document-sets/${setID}/workflow-operations`,
      { operation: "GENERATE_TESTCASES" },
      { "Idempotency-Key": `uv07-generate-${setID}` },
    );
    const cases = await until(
      () => api(`document-sets/${setID}/test-cases`),
      (v) => v.test_cases.length > 0,
    );
    const v1 = cases.test_cases[0];
    console.log(
      `Fixture set ${setID}, testcase ${v1.id}, family ${v1.family_id}`,
    );
    await page.goto(`${base}/documents/${setID}?step=test-cases`);
    await page
      .getByRole("heading", { name: "Testcase theo identity" })
      .waitFor();
    await page
      .getByRole("checkbox", { name: `Chọn ${v1.test_case_key} v1` })
      .check();
    await page
      .getByLabel("Tên người duyệt", { exact: true })
      .fill("Browser QA");
    await page
      .getByRole("button", { name: "Duyệt nhóm đã chọn", exact: true })
      .click();
    await page
      .getByRole("dialog", { name: "Xác nhận revision testcase" })
      .waitFor();
    await page.getByRole("button", { name: "Xác nhận duyệt revision" }).click();
    await page
      .getByText(`#${v1.id} v1: đã lưu APPROVED`, { exact: true })
      .waitFor();
    const publish = async (revision, number) => {
      await page.goto(`${base}/documents/${setID}?step=use-export`);
      await page
        .getByRole("heading", { name: "Chốt release bất biến" })
        .waitFor();
      await page
        .getByLabel("Tên người chốt", { exact: true })
        .fill("Browser QA");
      await page
        .getByLabel(`Release revision ${v1.test_case_key}`, { exact: true })
        .selectOption(String(revision));
      await page
        .getByLabel("Lý do phạm vi một phần", { exact: true })
        .fill("Selected scenario only");
      await page
        .getByRole("button", { name: "Xem trước phạm vi release" })
        .click();
      await page.getByRole("region", { name: "Preview release" }).waitFor();
      await page.screenshot({
        path: path.join(out, `release-${number}-preview.png`),
        fullPage: true,
      });
      await page.getByRole("button", { name: "Xác nhận chốt release" }).click();
      await page
        .locator("summary")
        .filter({ hasText: new RegExp(`^R${number} ·`) })
        .first()
        .waitFor();
    };
    await publish(v1.id, 1);
    await page.goto(`${base}/documents/${setID}/test-cases/${v1.id}`);
    await page
      .getByLabel("Tiêu đề", { exact: true })
      .fill("UV07 revised title");
    await page
      .getByLabel("Tác nhân", { exact: true })
      .fill("Reviewer customer");
    await page
      .getByLabel("Thao tác bước 1", { exact: true })
      .fill("Submit expired coupon from checkout");
    await page
      .getByLabel("Lý do / ghi chú", { exact: true })
      .fill("Clarify actor and first step");
    assert.ok(
      await page
        .getByRole("button", { name: "Duyệt phiên bản", exact: true })
        .isDisabled(),
    );
    await page.getByRole("button", { name: "Lưu bản nháp mới" }).click();
    await page.waitForURL(
      (u) =>
        /\/test-cases\/\d+$/.test(u.pathname) &&
        !u.pathname.endsWith(`/${v1.id}`),
    );
    const v2ID = Number(new URL(page.url()).pathname.split("/").at(-1));
    assert.equal((await api(`test-cases/${v2ID}`)).test_case.status, "DRAFT");
    await page.getByRole("button", { name: "Lịch sử & so sánh" }).click();
    await page
      .getByRole("table", {
        name: "Khác biệt nội dung theo thứ tự bước và citation",
      })
      .waitFor();
    assert.ok(
      (await page.getByRole("table").first().innerText()).includes(
        "Submit expired coupon from checkout",
      ),
    );
    await page.screenshot({
      path: path.join(out, "revision-diff.png"),
      fullPage: true,
    });
    await page.getByRole("button", { name: "Nội dung & duyệt" }).click();
    await page
      .getByLabel("Tên người duyệt", { exact: true })
      .fill("Browser QA");
    await page
      .getByRole("button", { name: "Duyệt phiên bản", exact: true })
      .click();
    await page
      .getByText(`Đã lưu quyết định cho v2 (#${v2ID}).`, { exact: true })
      .waitFor();
    await publish(v2ID, 2);
    const r2 = (
      await api(`document-sets/${setID}/suite-releases`)
    ).releases.find((r) => r.release_number === 2);
    await page.goto(`${base}/documents/${setID}/test-cases/${v2ID}`);
    await page.getByRole("button", { name: "Lịch sử & so sánh" }).click();
    await page
      .getByRole("button", { name: "Đối chiếu / phục hồi v1", exact: true })
      .click();
    await page
      .getByLabel("Lý do phục hồi / lưu trữ", { exact: true })
      .fill("Restore the first reviewed design");
    await page
      .getByRole("button", { name: "Phục hồi thành bản nháp mới" })
      .click();
    await page.waitForURL(
      (u) =>
        /\/test-cases\/\d+$/.test(u.pathname) &&
        !u.pathname.endsWith(`/${v2ID}`),
    );
    const v3ID = Number(new URL(page.url()).pathname.split("/").at(-1));
    const v3 = (await api(`test-cases/${v3ID}`)).test_case;
    assert.equal(v3.version_number, 3);
    assert.equal(v3.status, "DRAFT");
    assert.equal(v3.restored_from_revision_id, v1.id);
    assert.equal(
      (await api(`test-suite-releases/${r2.id}`)).manifest_hash,
      r2.manifest_hash,
    );
    await page.getByRole("button", { name: "Run & automation" }).click();
    await page
      .getByText(
        "Chưa chạy trong phạm vi này. Expected giống nhau không có nghĩa revision mới đã PASS.",
        { exact: true },
      )
      .waitFor();
    assert.equal((await api(`test-cases/${v3ID}/runs`)).runs.length, 0);
    await page.getByRole("button", { name: "Nội dung & duyệt" }).click();
    const download = page.waitForEvent("download");
    await page.getByRole("button", { name: "Xuất đúng testcase v3" }).click();
    const file = await download;
    await file.saveAs(path.join(out, file.suggestedFilename()));
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({
      path: path.join(out, "mobile-revision.png"),
      fullPage: true,
    });
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      "mobile overflow",
    );
    await page.keyboard.press("Tab");
    assert.ok(
      await page.evaluate(() => document.activeElement !== document.body),
    );
    await page.setViewportSize({ width: 1440, height: 1000 });
    const other = await browser.newPage();
    other.on("pageerror", (e) => errors.push(e.message));
    await other.goto(page.url());
    await page
      .getByLabel("Tiêu đề", { exact: true })
      .fill("Unsaved local title survives conflict");
    await page
      .getByLabel("Lý do / ghi chú", { exact: true })
      .fill("Keep local edits after comparison");
    await other
      .getByLabel("Tác nhân", { exact: true })
      .fill("Concurrent reviewer");
    await other
      .getByLabel("Lý do / ghi chú", { exact: true })
      .fill("Concurrent head update");
    await other.getByRole("button", { name: "Lưu bản nháp mới" }).click();
    await other.waitForURL(
      (u) =>
        /\/test-cases\/\d+$/.test(u.pathname) &&
        !u.pathname.endsWith(`/${v3ID}`),
    );
    await page.getByRole("button", { name: "Lưu bản nháp mới" }).click();
    const rebase = page.getByRole("button", {
      name: "Đã đối chiếu, dùng head mới làm nền và giữ form",
      exact: true,
    });
    await rebase.waitFor();
    assert.equal(
      await page.getByLabel("Tiêu đề", { exact: true }).inputValue(),
      "Unsaved local title survives conflict",
    );
    await page.screenshot({
      path: path.join(out, "concurrent-head-conflict.png"),
      fullPage: true,
    });
    await rebase.click();
    await page.getByRole("button", { name: "Lưu bản nháp mới" }).click();
    await page.waitForURL(
      (u) =>
        /\/test-cases\/\d+$/.test(u.pathname) &&
        !u.pathname.endsWith(`/${v3ID}`),
    );
    const v5ID = Number(new URL(page.url()).pathname.split("/").at(-1));
    assert.equal((await api(`test-cases/${v5ID}`)).test_case.version_number, 5);
    assert.equal(
      (await api(`test-suite-releases/${r2.id}`)).manifest_hash,
      r2.manifest_hash,
    );
    await other.close();
    await page.getByRole("button", { name: "Lịch sử & so sánh" }).click();
    await page
      .getByLabel("Lý do phục hồi / lưu trữ", { exact: true })
      .fill("Archive without removing history");
    page.once("dialog", (dialog) => dialog.accept());
    await page
      .getByRole("button", { name: "Lưu trữ identity", exact: true })
      .click();
    await page
      .getByText("Đã lưu trữ identity; lịch sử/release/run vẫn giữ nguyên.", {
        exact: true,
      })
      .waitFor();
    assert.equal(
      (await api(`test-case-families/${v1.family_id}/history`)).entries.length,
      5,
    );
    assert.deepEqual(errors, []);
    console.log(
      JSON.stringify(
        {
          status: "passed",
          setID,
          family: v1.family_id,
          v1: v1.id,
          v2: v2ID,
          v3: v3ID,
          release: r2.id,
          artifacts: out,
          checks: [
            "exact batch approval",
            "R1 preview/publish",
            "save draft separately",
            "ordered field/step diff",
            "approve v2",
            "R2",
            "restore v1 as v3 draft",
            "R2 unchanged",
            "no inherited runs",
            "revision export",
            "390px",
            "keyboard",
            "concurrent head 409 preserves local draft",
            "explicit rebase creates v5",
            "archive retains history",
            "no pageerror",
          ],
        },
        null,
        2,
      ),
    );
  } catch (error) {
    await page.screenshot({
      path: path.join(out, "failure.png"),
      fullPage: true,
    });
    console.error({ setID, url: page.url(), error: error.message, errors });
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
})();
