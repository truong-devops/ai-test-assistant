// Retained fixtures only on an isolated schema-30 API/worker/UI, provider disabled.
// Never run against production. Setup/review/release proof uses API; generation
// scope, proposal diff and decisions run in the browser.
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
  const base = process.env.UV08_BASE_URL || "http://127.0.0.1:3188";
  const out = process.env.UV08_ARTIFACT_DIR || "/tmp/uv08-proposal-ui-evidence";
  fs.mkdirSync(out, { recursive: true });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const api = async (route, data, key) => {
    const url = `${base}/api/backend/api/${route}`;
    const r =
      data === undefined
        ? await page.request.get(url)
        : await page.request.post(url, {
            data,
            headers: key ? { "Idempotency-Key": key } : {},
          });
    assert.ok(r.ok(), `${route}: ${r.status()} ${await r.text()}`);
    return r.json();
  };
  const until = async (fn, predicate) => {
    const end = Date.now() + 90000;
    while (Date.now() < end) {
      const v = await fn();
      if (predicate(v)) return v;
      await new Promise((r) => setTimeout(r, 700));
    }
    throw new Error("Durable state timed out");
  };
  try {
    const set = await api("document-sets", {
      name: `UV08 proposals browser ${Date.now()}`,
    });
    const id = set.id;
    const uploaded = await page.request.post(
      `${base}/api/backend/api/document-sets/${id}/documents`,
      {
        multipart: {
          new_document: "true",
          file: {
            name: "coupon.md",
            mimeType: "text/markdown",
            buffer: Buffer.from(
              "# Coupon requirements\n\n| Mã YC | Yêu cầu |\n| --- | --- |\n| REQ-101 | The system must reject expired coupons and show a validation error. |\n",
            ),
          },
        },
      },
    );
    assert.equal(uploaded.status(), 202);
    const docs = await until(
      () => api(`document-sets/${id}/documents`),
      (v) => v.documents[0]?.latest_version.parse_status === "PARSED",
    );
    const version = docs.documents[0].latest_version;
    const workflow = await api(`document-sets/${id}/workflow`);
    await api(
      `document-sets/${id}/source-review`,
      {
        command: "APPROVE_AND_EXTRACT",
        expected_source_revision: workflow.source_revision,
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
      `source-${id}`,
    );
    await until(
      () => api(`document-sets/${id}/workflow`),
      (v) =>
        v.source_intents.some(
          (i) =>
            i.command === "APPROVE_AND_EXTRACT" && i.status === "SUCCEEDED",
        ),
    );
    const req = (await api(`document-sets/${id}/requirements`)).requirements[0];
    await api(
      `document-sets/${id}/requirement-review/bulk-review`,
      {
        items: [{ id: req.id, expected_hash: req.review_hash }],
        decision: "APPROVED",
        reviewer_name: "Fixture QA",
      },
      `req-${id}`,
    );
    await page.goto(`${base}/documents/${id}?step=test-cases`);
    const workspace = page.getByRole("region", {
      name: "Sinh và đối chiếu đề xuất",
      exact: true,
    });
    await workspace
      .getByRole("heading", { name: "Sinh và đối chiếu đề xuất" })
      .waitFor();
    assert.ok(
      await workspace
        .getByRole("button", { name: "Sinh đề xuất để review", exact: true })
        .isDisabled(),
    );
    let latestJob = 0;
    const generate = async (scope) => {
      console.log(`Generate ${scope} after job ${latestJob}`);
      await workspace
        .getByLabel("Phạm vi sinh đề xuất", { exact: true })
        .selectOption(scope);
      if (scope === "selected") {
        const box = workspace
          .getByRole("group", { name: "Chọn yêu cầu cho lần chạy này" })
          .getByRole("checkbox")
          .first();
        await box.check();
      }
      await workspace
        .getByRole("checkbox", { name: /^Tôi xác nhận sinh đề xuất/ })
        .check();
      const sent = page.waitForRequest(
        (r) =>
          r.method() === "POST" &&
          r.url().endsWith(`/document-sets/${id}/workflow-operations`),
      );
      const [request] = await Promise.all([
        sent,
        workspace
          .getByRole("button", { name: "Sinh đề xuất để review", exact: true })
          .click(),
      ]);
      const body = request.postDataJSON();
      assert.equal(body.review_proposals, true);
      assert.equal(body.generation_scope, scope.toUpperCase());
      if (scope === "selected")
        assert.deepEqual(body.requirement_ids, [req.id]);
      const result = await until(
        () => api(`document-sets/${id}/workflow`),
        (v) =>
          v.recent_jobs.some(
            (j) =>
              j.operation === "GENERATE_TESTCASES" &&
              j.id > latestJob &&
              j.status === "SUCCEEDED",
          ),
      );
      latestJob = result.recent_jobs.find(
        (j) => j.operation === "GENERATE_TESTCASES",
      ).id;
      const proposals = (
        await api(`document-sets/${id}/test-case-proposals?job_id=${latestJob}`)
      ).proposals;
      await workspace
        .getByRole("button", {
          name: `Xem đề xuất #${proposals[0].id}`,
          exact: true,
        })
        .waitFor();
      return proposals;
    };
    const open = async (p) => {
      await workspace
        .getByRole("button", { name: `Xem đề xuất #${p.id}`, exact: true })
        .click();
      const review = workspace.getByRole("region", {
        name: `Review đề xuất #${p.id}`,
        exact: true,
      });
      await review
        .getByText(new RegExp(`Job #${p.workflow_job_id}: SUCCEEDED`))
        .waitFor();
      return review;
    };
    const choose = async (review, decision, reason) => {
      await review
        .getByLabel("Cách xử lý đề xuất", { exact: true })
        .selectOption(decision);
      await review.getByLabel("Lý do quyết định", { exact: true }).fill(reason);
      await review
        .getByRole("checkbox", { name: /^Tôi đã đọc nội dung/ })
        .check();
    };
    const first = (await generate("all")).find(
      (p) => p.classification === "NEW_CASE",
    );
    assert.ok(first);
    assert.equal(
      (await api(`document-sets/${id}/test-cases`)).test_cases.length,
      0,
    );
    let review = await open(first);
    await choose(review, "CREATE_NEW", "Browser reviewed new scenario");
    // Commit upstream, drop the browser response once; retry must reuse its key.
    const keys = [];
    const route = `**/api/backend/api/test-case-proposals/${first.id}/review`;
    await page.route(route, async (r) => {
      keys.push(r.request().headers()["idempotency-key"]);
      const upstream = await r.fetch();
      if (keys.length === 1) await r.abort("failed");
      else await r.fulfill({ response: upstream });
    });
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review.getByRole("alert").waitFor();
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("status")
      .filter({ hasText: "Đã ghi nhận" })
      .waitFor();
    assert.equal(keys.length, 2);
    assert.equal(keys[0], keys[1]);
    await page.unroute(route);
    let cases = (await api(`document-sets/${id}/test-cases`)).test_cases;
    assert.equal(cases.length, 1);
    const v1 = cases[0];
    assert.equal(v1.status, "DRAFT");
    assert.equal(v1.automation_status, "MANUAL");
    await api(`test-cases/${v1.id}/review`, {
      decision: "APPROVED",
      reviewer_name: "Fixture QA",
      expected_content_hash: v1.content_hash,
    });
    const r1 = await api(
      `document-sets/${id}/suite-releases`,
      {
        test_suite_id: v1.test_suite_id,
        revision_ids: [v1.id],
        published_by: "Fixture QA",
        scope_decision: "Selected browser scenario",
      },
      `r1-${id}`,
    );
    const exported = await api(`document-sets/${id}/exports`, {
      test_suite_id: v1.test_suite_id,
      suite_release_id: r1.id,
      format: "XLSX",
      generated_by: "Fixture QA",
    });
    const download = async () => {
      const response = await page.request.get(
        `${base}/api/backend${exported.download_url}`,
      );
      assert.ok(response.ok());
      if (process.env.UV09_TEST_TOKEN) {
        assert.ok(!JSON.stringify(response.headers()).includes(process.env.UV09_TEST_TOKEN));
        assert.ok(!(await response.body()).includes(Buffer.from(process.env.UV09_TEST_TOKEN)));
      }
      return response.body();
    };
    const r1Bytes = await download();
    const unchanged = (await generate("selected")).find(
      (p) => p.classification === "UNCHANGED",
    );
    assert.ok(unchanged);
    review = await open(unchanged);
    await review
      .getByText("Không có khác biệt nội dung.", { exact: true })
      .waitFor();
    await choose(review, "KEEP", "Exact content and context unchanged");
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("status")
      .filter({ hasText: "Đã ghi nhận" })
      .waitFor();
    assert.equal(
      (await api(`document-sets/${id}/test-cases`)).test_cases.length,
      1,
    );
    // Change human head after the proposal has pinned v1; do not silently rebase.
    const stale = (await generate("selected")).find(
      (p) => p.classification === "UNCHANGED",
    );
    review = await open(stale);
    await api(
      `test-case-families/${v1.family_id}/versions`,
      {
        base_revision_id: v1.id,
        expected_head_revision_id: v1.id,
        patch: { title: "Human head must survive" },
        reason: "Concurrent human edit",
      },
      `human-${id}`,
    );
    await choose(review, "KEEP", "Keep my review reason on conflict");
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("alert")
      .filter({ hasText: "không tự rebase" })
      .waitFor();
    assert.equal(
      await review.getByLabel("Lý do quyết định", { exact: true }).inputValue(),
      "Keep my review reason on conflict",
    );
    assert.ok(
      await review
        .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
        .isDisabled(),
    );
    await page.screenshot({
      path: path.join(out, "head-conflict.png"),
      fullPage: true,
    });
    await choose(review, "DISMISS", "Superseded by human edit");
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("status")
      .filter({ hasText: "Đã ghi nhận" })
      .waitFor();
    const ambiguous = (await generate("selected")).find(
      (p) => p.classification === "AMBIGUOUS_MATCH",
    );
    assert.ok(ambiguous);
    review = await open(ambiguous);
    await review.getByRole("table").waitFor();
    await choose(
      review,
      "REVISE",
      "Explicitly confirmed same business identity",
    );
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("status")
      .filter({ hasText: "Đã ghi nhận" })
      .waitFor();
    cases = (await api(`document-sets/${id}/test-cases`)).test_cases;
    assert.equal(cases.length, 1);
    assert.equal(cases[0].version_number, 3);
    cases = (await api(`test-case-families/${v1.family_id}/versions`)).versions;
    assert.equal(cases.length, 3);
    assert.equal(
      cases.find((c) => c.version_number === 2).title,
      "Human head must survive",
    );
    assert.equal(cases.find((c) => c.version_number === 3).status, "DRAFT");
    assert.equal(
      cases.find((c) => c.version_number === 3).automation_status,
      "MANUAL",
    );
    assert.ok(!cases.find((c) => c.version_number === 3).latest_execution);
    const releases = (await api(`document-sets/${id}/suite-releases`)).releases;
    assert.equal(releases.length, 1);
    assert.equal(releases[0].manifest_hash, r1.manifest_hash);
    assert.equal(releases[0].items[0].test_case_id, v1.id);
    await page.screenshot({
      path: path.join(out, "proposal-applied.png"),
      fullPage: true,
    });
    // Exercise a real new document version, extraction, affected-scope proposal,
    // exact revision review and release R2 (not a mocked source-state update).
    const updated = await page.request.post(
      `${base}/api/backend/api/document-sets/${id}/documents/${docs.documents[0].id}/versions`,
      {
        multipart: {
          file: {
            name: "coupon.md",
            mimeType: "text/markdown",
            buffer: Buffer.from(
              "# Coupon requirements\n\n| Mã YC | Yêu cầu |\n| --- | --- |\n| REQ-101 | The system must reject expired coupons and retain cart contents. |\n",
            ),
          },
        },
      },
    );
    assert.equal(updated.status(), 202, await updated.text());
    const changedDocs = await until(
      () => api(`document-sets/${id}/documents`),
      (v) =>
        v.documents[0]?.latest_version.id !== version.id &&
        v.documents[0]?.latest_version.parse_status === "PARSED",
    );
    const changedVersion = changedDocs.documents[0].latest_version;
    const changedWorkflow = await api(`document-sets/${id}/workflow`);
    const lastIntent = Math.max(
      ...changedWorkflow.source_intents.map((i) => i.id),
    );
    await api(
      `document-sets/${id}/source-review`,
      {
        command: "APPROVE_AND_EXTRACT",
        expected_source_revision: changedWorkflow.source_revision,
        selected: [
          {
            version_id: changedVersion.id,
            sha256: changedVersion.sha256,
            approval_status: changedVersion.approval_status,
          },
        ],
        excluded_version_ids: [],
        reviewer_name: "Fixture QA",
      },
      `source-update-${id}`,
    );
    await until(
      () => api(`document-sets/${id}/workflow`),
      (v) =>
        v.source_intents.some(
          (i) =>
            i.id > lastIntent &&
            i.command === "APPROVE_AND_EXTRACT" &&
            i.status === "SUCCEEDED",
        ),
    );
    const changedReq = (
      await api(`document-sets/${id}/requirements`)
    ).requirements.find((r) => r.source_state === "CURRENT");
    assert.ok(changedReq && changedReq.id !== req.id);
    await api(
      `document-sets/${id}/requirement-review/bulk-review`,
      {
        items: [{ id: changedReq.id, expected_hash: changedReq.review_hash }],
        decision: "APPROVED",
        reviewer_name: "Fixture QA",
      },
      `req-update-${id}`,
    );
    await page.goto(`${base}/documents/${id}?step=test-cases`);
    assert.equal(
      await workspace
        .getByLabel("Phạm vi sinh đề xuất", { exact: true })
        .inputValue(),
      "affected",
    );
    const changedProposals = await generate("affected");
    assert.equal(changedProposals.length, 1);
    const changedProposal = changedProposals[0];
    assert.equal(changedProposal.classification, "AMBIGUOUS_MATCH");
    review = await open(changedProposal);
    await review.getByRole("table").waitFor();
    await choose(
      review,
      "REVISE",
      "Confirmed same scenario after document update",
    );
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("status")
      .filter({ hasText: "Đã ghi nhận" })
      .waitFor();
    const v4 = (await api(`document-sets/${id}/test-cases`)).test_cases[0];
    assert.equal(v4.version_number, 4);
    assert.equal(v4.status, "DRAFT");
    assert.ok(!v4.latest_execution);
    await page.goto(`${base}/documents/${id}/test-cases/${v4.id}`);
    await page
      .getByLabel("Tên người duyệt", { exact: true })
      .fill("Browser QA");
    await page
      .getByRole("button", { name: "Duyệt phiên bản", exact: true })
      .click();
    await page
      .getByText(`Đã lưu quyết định cho v4 (#${v4.id}).`, { exact: true })
      .waitFor();
    await page.goto(`${base}/documents/${id}?step=use-export`);
    await page
      .getByRole("heading", { name: "Chốt release bất biến" })
      .waitFor();
    await page.getByLabel("Tên người chốt", { exact: true }).fill("Browser QA");
    await page
      .getByLabel(`Release revision ${v1.test_case_key}`, { exact: true })
      .selectOption(String(v4.id));
    await page
      .getByLabel("Lý do phạm vi một phần", { exact: true })
      .fill("Updated source scenario");
    await page
      .getByRole("button", { name: "Xem trước phạm vi release" })
      .click();
    await page.getByRole("region", { name: "Preview release" }).waitFor();
    await page.getByRole("button", { name: "Xác nhận chốt release" }).click();
    await page
      .locator("summary")
      .filter({ hasText: /^R2 ·/ })
      .first()
      .waitFor();
    const afterReleases = (await api(`document-sets/${id}/suite-releases`))
      .releases;
    assert.equal(afterReleases.length, 2);
    assert.equal(
      afterReleases.find((r) => r.id === r1.id).manifest_hash,
      r1.manifest_hash,
    );
    assert.deepEqual(afterReleases.find((r) => r.id === r1.id).items, r1.items);
    assert.equal(
      afterReleases.find((r) => r.release_number === 2).items[0].test_case_id,
      v4.id,
    );
    assert.deepEqual(await download(), r1Bytes);
    await page.screenshot({
      path: path.join(out, "source-update-r2.png"),
      fullPage: true,
    });
    await page.goto(`${base}/documents/${id}/test-cases`);
    await page
      .getByRole("heading", { name: "Sinh và đối chiếu đề xuất" })
      .waitFor();
    await page.setViewportSize({ width: 390, height: 844 });
    await page
      .getByRole("button", {
        name: `Xem đề xuất #${ambiguous.id}`,
        exact: true,
      })
      .click();
    await page
      .getByRole("region", {
        name: `Review đề xuất #${ambiguous.id}`,
        exact: true,
      })
      .getByRole("table")
      .waitFor();
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await page.screenshot({
      path: path.join(out, "mobile.png"),
      fullPage: true,
    });
    if (process.env.UV08_VIEWER_URL) {
      const viewerBase = process.env.UV08_VIEWER_URL;
      const viewer = await browser.newPage();
      viewer.on("pageerror", (e) => errors.push(e.message));
      await viewer.goto(`${viewerBase}/documents/${id}?step=test-cases`);
      const viewerWorkspace = viewer.getByRole("region", {
        name: "Sinh và đối chiếu đề xuất",
        exact: true,
      });
      await viewerWorkspace
        .getByRole("heading", { name: "Sinh và đối chiếu đề xuất" })
        .waitFor();
      assert.ok(
        await viewerWorkspace
          .getByLabel("Phạm vi sinh đề xuất", { exact: true })
          .isDisabled(),
      );
      await viewerWorkspace
        .getByRole("button", {
          name: `Xem đề xuất #${ambiguous.id}`,
          exact: true,
        })
        .click();
      const readonly = viewerWorkspace.getByRole("region", {
        name: `Review đề xuất #${ambiguous.id}`,
        exact: true,
      });
      await readonly.getByRole("table").waitFor();
      assert.equal(
        await readonly
          .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
          .count(),
        0,
      );
      const denied = await viewer.request.post(
        `${viewerBase}/api/backend/api/test-case-proposals/${ambiguous.id}/review`,
        {
          data: { decision: "DISMISS", reason: "must be denied" },
          headers: { "Idempotency-Key": "viewer-forbidden" },
        },
      );
      assert.equal(denied.status(), 403);
      await viewer.screenshot({
        path: path.join(out, "viewer-readonly.png"),
        fullPage: true,
      });
      await viewer.close();
    }
    // No current approved requirements left: the retire-only path must still be
    // available, archive explicitly, and preserve both published releases.
    await page.setViewportSize({ width: 1440, height: 1000 });
    const removedUpload = await page.request.post(
      `${base}/api/backend/api/document-sets/${id}/documents/${docs.documents[0].id}/versions`,
      {
        multipart: {
          file: {
            name: "coupon.md",
            mimeType: "text/markdown",
            buffer: Buffer.from("# Overview\n"),
          },
        },
      },
    );
    assert.equal(removedUpload.status(), 202);
    const removedDocs = await until(
      () => api(`document-sets/${id}/documents`),
      (v) =>
        v.documents[0]?.latest_version.id !== changedVersion.id &&
        v.documents[0]?.latest_version.parse_status === "PARSED",
    );
    const removedVersion = removedDocs.documents[0].latest_version;
    const beforeRemoval = await api(`document-sets/${id}/workflow`);
    const lastRemovalIntent = Math.max(
      ...beforeRemoval.source_intents.map((i) => i.id),
    );
    await api(
      `document-sets/${id}/source-review`,
      {
        command: "APPROVE_AND_EXTRACT",
        expected_source_revision: beforeRemoval.source_revision,
        selected: [
          {
            version_id: removedVersion.id,
            sha256: removedVersion.sha256,
            approval_status: removedVersion.approval_status,
          },
        ],
        excluded_version_ids: [],
        reviewer_name: "Fixture QA",
      },
      `source-removed-${id}`,
    );
    await until(
      () => api(`document-sets/${id}/workflow`),
      (v) =>
        v.source_intents.some(
          (i) =>
            i.id > lastRemovalIntent &&
            i.command === "APPROVE_AND_EXTRACT" &&
            i.status === "SUCCEEDED",
        ),
    );
    assert.equal(
      (await api(`document-sets/${id}/requirements`)).requirements.filter(
        (r) => r.source_state === "CURRENT",
      ).length,
      0,
    );
    await page.goto(`${base}/documents/${id}?step=test-cases`);
    const retire = await generate("affected");
    assert.equal(retire.length, 1);
    assert.equal(retire[0].classification, "RETIRE_CANDIDATE");
    review = await open(retire[0]);
    await choose(
      review,
      "ARCHIVE",
      "Confirmed the last source requirement was removed",
    );
    await review
      .getByRole("button", { name: "Xác nhận xử lý đề xuất", exact: true })
      .click();
    await review
      .getByRole("status")
      .filter({ hasText: "Đã ghi nhận" })
      .waitFor();
    const archivedReleases = (await api(`document-sets/${id}/suite-releases`))
      .releases;
    assert.deepEqual(
      archivedReleases.map((r) => r.manifest_hash),
      afterReleases.map((r) => r.manifest_hash),
    );
    assert.deepEqual(await download(), r1Bytes);
    assert.equal(
      (await api(`test-case-families/${v1.family_id}/versions`)).versions
        .length,
      4,
    );
    await page.screenshot({
      path: path.join(out, "all-removed-archive.png"),
      fullPage: true,
    });
    assert.deepEqual(errors, []);
    console.log(
      JSON.stringify({
        setID: id,
        requirementID: req.id,
        v1: v1.id,
        latestJob,
        r1,
        result: "PASS",
      }),
    );
  } catch (e) {
    await page.screenshot({
      path: path.join(out, "failure.png"),
      fullPage: true,
    });
    throw e;
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
