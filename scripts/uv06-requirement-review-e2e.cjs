// Isolated API + worker + production frontend, LLM_PROVIDER=disabled, schema 28.
// Creates and retains its own fixtures. Do not run against production.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1050 } });
  page.setDefaultTimeout(30000);
  const base = process.env.UV06_BASE_URL || 'http://127.0.0.1:3186';
  const out = process.env.UV06_ARTIFACT_DIR || '/tmp/uv06-browser-evidence';
  fs.mkdirSync(out, { recursive: true });
  const errors = []; page.on('pageerror', (error) => errors.push(error.message));
  const api = async (route, data, extra = {}) => {
    const response = data === undefined ? await page.request.get(`${base}/api/backend/api/${route}`) : await page.request.post(`${base}/api/backend/api/${route}`, { data, ...extra });
    assert.ok(response.ok(), `${route}: ${response.status()} ${await response.text()}`);
    return response.json();
  };
  const until = async (load, predicate, timeout = 90000) => {
    const end = Date.now() + timeout;
    while (Date.now() < end) { const value = await load(); if (predicate(value)) return value; await new Promise((resolve) => setTimeout(resolve, 700)); }
    throw new Error('Timed out waiting for durable state');
  };
  let setID;
  try {
    const set = await api('document-sets', { name: `UV06 browser ${Date.now()}` }); setID = set.id;
    // Distinct business statements avoid the extractor's semantic deduplication.
    const rules = ['Customers must verify email addresses before registration.', 'Administrators must configure warehouse opening hours.', 'Payments must reject expired credit cards.', 'Shipments must include tracking numbers.', 'Invoices must show tax amounts separately.', 'Refunds must return funds to the original account.', 'Passwords must contain twelve characters.', 'Sessions must expire after thirty minutes.', 'Catalog searches must support exact product names.', 'Inventory adjustments must record employee identity.', 'Discount codes must expire on their configured date.', 'Reports must export monthly revenue as CSV.', 'Notifications must include the delivery address.', 'Audit logs must retain every permission change.', 'Attachments must reject executable files.', 'Reservations must release seats after cancellation.', 'Subscriptions must renew on the billing anniversary.', 'Reviews must hide offensive language.', 'Currencies must display two decimal places.', 'Backups must encrypt stored customer records.'];
    const source = rules.map((rule, i) => `# REQ-${String(i + 1).padStart(3, '0')}\n\n${rule}\n`).join('\n') + '\n# REQ-TBD\n\nThe allowed order limit is TBD and must be clarified.\n';
    const uploaded = await page.request.post(`${base}/api/backend/api/document-sets/${setID}/documents`, { multipart: { new_document: 'true', file: { name: 'rules.md', mimeType: 'text/markdown', buffer: Buffer.from(source) } } });
    assert.equal(uploaded.status(), 202);
    const upload = await uploaded.json();
    const prepared = await until(() => api(`document-sets/${setID}/documents`), (value) => value.documents[0]?.latest_version.parse_status === 'PARSED');
    const v = prepared.documents[0].latest_version;
    const workflow = await api(`document-sets/${setID}/workflow`);
    await api(`document-sets/${setID}/source-review`, { command: 'APPROVE_AND_EXTRACT', expected_source_revision: workflow.source_revision, selected: [{ version_id: v.id, sha256: v.sha256, approval_status: v.approval_status }], excluded_version_ids: [], reviewer_name: 'Browser QA' }, { headers: { 'Idempotency-Key': `source-${setID}` } });
    await until(() => api(`document-sets/${setID}/workflow`), (value) => value.source_intents.some((item) => item.command === 'APPROVE_AND_EXTRACT' && item.status === 'SUCCEEDED'));
    let inventory = await api(`document-sets/${setID}/requirements`);
    assert.ok(inventory.requirements.filter((item) => item.status === 'DRAFT').length >= 20, 'fixture needs 20 eligible drafts');
    console.log(`Workspace ${setID}: ${inventory.requirements.length} requirements extracted`);
    await page.goto(`${base}/documents/${setID}?step=requirements`);
    await page.getByRole('heading', { name: 'Requirement inventory', exact: true }).waitFor();
    await page.getByLabel('Status filter').selectOption('DRAFT');
    assert.equal(await page.locator('tbody input[type=checkbox]:checked').count(), 0);
    await page.getByRole('button', { name: 'Xem evidence & duyệt' }).first().click();
    await page.locator('.source-preview .evidence-drawer article').first().waitFor();
    await page.getByRole('button', { name: 'Đóng evidence, giữ danh sách' }).click();
    assert.equal(await page.getByLabel('Status filter').inputValue(), 'DRAFT');
    await page.getByRole('button', { name: 'Chọn các mục trên trang này' }).click();
    assert.equal(await page.locator('tbody input[type=checkbox]:checked').count(), 20);
    await page.getByLabel('Tên hiển thị người duyệt', { exact: true }).fill('Browser Reviewer');
    await page.getByRole('button', { name: 'Duyệt nhóm đã chọn', exact: true }).click();
    await page.getByRole('dialog', { name: 'Xác nhận duyệt nhóm' }).waitFor();
    await page.screenshot({ path: path.join(out, 'batch-confirmation.png'), fullPage: true });
    await page.getByRole('button', { name: 'Xác nhận lưu quyết định' }).click();
    await page.getByText('Đã lưu 20/20 quyết định.', { exact: true }).waitFor();
    await page.reload();
    await page.waitForFunction(() => document.querySelector('[aria-label="Status filter"]')?.value === 'DRAFT');
    assert.equal(await page.getByLabel('Status filter').inputValue(), 'DRAFT');
    inventory = await api(`document-sets/${setID}/requirements`);
    assert.equal(inventory.requirements.filter((item) => item.status === 'APPROVED').length, 20);
    console.log('20 exact drafts reviewed, persisted across reload');
    await page.getByRole('button', { name: /Cần làm rõ ·/ }).click();
    await page.getByRole('button', { name: 'Trả lời câu hỏi' }).first().click();
    await page.getByLabel(/Nội dung làm rõ/).fill('The maximum order size is 20 units. Larger orders must be rejected.');
    await page.getByRole('button', { name: 'Lưu nội dung làm rõ' }).click();
    await page.getByRole('button', { name: 'Lưu nội dung làm rõ' }).waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: 'Cần làm rõ · 0', exact: true }).waitFor();
    await page.locator('article.clarification-item p').filter({ hasText: 'The maximum order size is 20 units. Larger orders must be rejected.' }).waitFor();
    const questions = await api(`document-sets/${setID}/open-questions`);
    assert.equal(questions.open_questions.filter((item) => item.status === 'OPEN').length, 0);
    await page.getByRole('button', { name: 'Danh sách', exact: true }).click();
    await page.getByLabel('Status filter').selectOption('APPROVED');
    const original = inventory.requirements.find((item) => item.status === 'APPROVED');
    await page.goto(`${base}/documents/${setID}/requirements/${original.id}`);
    await page.getByLabel('Reviewer', { exact: true }).fill('Browser Reviewer');
    await page.getByLabel('Title', { exact: true }).fill(`${original.title} reviewed edit`);
    const reviewResponse = page.waitForResponse((response) => response.url().endsWith(`/requirements/${original.id}/review`) && response.request().method() === 'POST');
    await page.getByRole('button', { name: 'Accept / save edit' }).click();
    const reviewed = await reviewResponse;
    assert.equal(reviewed.status(), 200);
    await page.waitForURL((url) => /\/requirements\/\d+$/.test(url.pathname) && !url.pathname.endsWith(`/${original.id}`));
    const revisedID = Number(new URL(page.url()).pathname.split('/').at(-1));
    const oldDetail = await api(`requirements/${original.id}`); const newDetail = await api(`requirements/${revisedID}`);
    assert.equal(oldDetail.requirement.title, original.title); assert.equal(newDetail.requirement.supersedes_requirement_id, original.id);
    await page.getByRole('link', { name: 'Quay lại danh sách duyệt nhóm' }).click();
    await page.getByRole('heading', { name: 'Requirement inventory', exact: true }).waitFor();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: path.join(out, 'mobile-review.png'), fullPage: true });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1));
    await page.getByRole('button', { name: 'Đối chiếu nguồn', exact: true }).click();
    await page.getByRole('heading', { name: 'Đối chiếu nguồn cũ / mới' }).waitFor();
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ status: 'passed', setID, sourceDocumentID: upload.document?.id, revisedID, artifacts: out, checks: ['20-item batch', 'evidence preview', 'no default selection', 'filter persistence', 'reload', 'TBD resolution and count', 'edit redirect and old proof', '390px', 'source comparison', 'no browser errors'] }, null, 2));
  } catch (error) { await page.screenshot({ path: path.join(out, 'failure.png'), fullPage: true }); console.error(JSON.stringify({ setID, url: page.url(), error: error.message, browserErrors: errors }, null, 2)); process.exitCode = 1; }
  finally { await browser.close(); }
})();
