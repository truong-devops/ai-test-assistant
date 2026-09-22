// Run against an isolated API/worker/frontend stack with LLM_PROVIDER=disabled.
// PLAYWRIGHT_MODULE may point at an existing local Playwright installation.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1050 } });
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const base = process.env.UV05_BASE_URL || 'http://127.0.0.1:3185';
  const out = process.env.UV05_ARTIFACT_DIR || '/tmp/uv05-browser-evidence';
  fs.mkdirSync(out, { recursive: true });
  let setID;
  try {
    await page.goto(`${base}/documents`);
    await page.getByLabel('Tên bộ tài liệu', { exact: true }).fill(`UV05 browser ${Date.now()}`);
    await page.getByRole('button', { name: 'Tạo và mở bộ tài liệu' }).click();
    await page.waitForURL(/\/documents\/\d+\?step=documents/);
    setID = Number(new URL(page.url()).pathname.split('/').at(-1));
    await page.getByRole('heading', { name: 'Chưa có tài liệu' }).waitFor();
    console.log(`Workspace created: ${setID}`);
    assert.equal(await page.getByRole('navigation', { name: 'Các bước từ tài liệu đến testcase' }).getByRole('link').count(), 4);
    await page.locator('input[type=file]').setInputFiles([
      { name: 'checkout.md', mimeType: 'text/markdown', buffer: Buffer.from('# REQ-CHECKOUT\n\nThe system must create an order when a customer confirms checkout.\n\n| Field | Required |\n| --- | --- |\n| Address | Yes |\n') },
      { name: 'broken.docx', mimeType: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', buffer: Buffer.from('not a DOCX archive') },
    ]);
    await page.getByRole('button', { name: 'Tải tài liệu lên', exact: true }).click();
    const rows = page.getByRole('table').first().getByRole('row');
    const good = rows.filter({ hasText: 'checkout.md' });
    const bad = rows.filter({ hasText: 'broken.docx' });
    await good.getByText('Đã đọc', { exact: false }).waitFor({ timeout: 60000 });
    await bad.getByText('Không đọc được', { exact: false }).first().waitFor({ timeout: 60000 });
    console.log('Mixed parse outcomes visible without reload');
    assert.equal(await page.locator('input[type=checkbox]:checked').count(), 0);
    await good.getByRole('button', { name: 'Xem & duyệt' }).click();
    await page.locator('.source-preview').getByRole('heading', { name: 'checkout · v1' }).waitFor();
    await page.locator('.source-preview').getByRole('table').waitFor();
    await bad.getByRole('checkbox', { name: 'Loại khỏi lần xử lý này' }).check();
    await good.getByRole('checkbox', { name: 'Chọn duyệt checkout v1' }).check();
    await page.getByLabel('Tên hiển thị người duyệt').fill('Browser QA');
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: path.join(out, 'desktop-source-review.png'), fullPage: true });
    await page.getByRole('button', { name: 'Duyệt nguồn & trích xuất yêu cầu bằng AI' }).click();
    await page.getByText('Đã lưu yêu cầu.', { exact: false }).waitFor();
    await page.reload();
    await page.getByText('Phạm vi đã lưu loại 1 phiên bản:', { exact: false }).waitFor();
    // Poll only read endpoints; closing/reopening the page must not create a new command.
    await page.waitForFunction(async (id) => {
      const response = await fetch(`/api/backend/api/document-sets/${id}/workflow`);
      const value = await response.json();
      return value.source_intents.some((intent) => intent.command === 'APPROVE_AND_EXTRACT' && intent.status === 'SUCCEEDED');
    }, setID, { timeout: 60000, polling: 1000 });
    console.log('Saved source command completed after reload');
    await page.getByRole('navigation', { name: 'Các bước từ tài liệu đến testcase' }).getByRole('link').filter({ hasText: 'Yêu cầu' }).click();
    await page.getByRole('heading', { name: 'Requirement inventory' }).waitFor();
    console.log('Requirements navigation ready');
    assert.ok(await page.locator('tbody tr').count() > 0);
    await page.goto(`${base}/documents/${setID}?step=documents`);
    const sourceRow = page.getByRole('table').first().getByRole('row').filter({ hasText: 'checkout.md' });
    await sourceRow.getByRole('button', { name: 'Tải phiên bản mới' }).click();
    await page.getByRole('heading', { name: 'Phiên bản mới · checkout' }).waitFor();
    await page.locator('input[type=file]').setInputFiles({ name: 'renamed-v2.md', mimeType: 'text/markdown', buffer: Buffer.from('# REQ-CHECKOUT\n\nThe system must create an order and return its confirmation number.\n') });
    await page.getByRole('button', { name: 'Tải tài liệu lên', exact: true }).click();
    const v2Row = page.getByRole('table').first().getByRole('row').filter({ hasText: 'renamed-v2.md' });
    await v2Row.getByText('v2 · renamed-v2.md').waitFor();
    await v2Row.getByRole('button', { name: 'Lịch sử nguồn' }).click();
    await page.getByRole('button', { name: 'Xem v1 · Đã duyệt' }).click();
    await page.getByText('Đang xem lịch sử.', { exact: false }).waitFor();
    await page.getByRole('link', { name: 'Mở trang phiên bản' }).click();
    await page.getByRole('link', { name: 'Quay lại danh sách để chọn và duyệt nguồn' }).waitFor();
    await page.getByRole('link', { name: 'Quay lại danh sách để chọn và duyệt nguồn' }).click();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole('heading', { name: 'Nguồn tài liệu', exact: true }).waitFor();
    await page.getByRole('table').first().getByRole('row').filter({ hasText: 'renamed-v2.md' }).waitFor();
    await page.screenshot({ path: path.join(out, 'mobile-workspace.png'), fullPage: true });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), 'workspace overflows mobile viewport');
    // Keyboard navigation reaches the first business step and activates it.
    await page.getByRole('navigation', { name: 'Các bước từ tài liệu đến testcase' }).getByRole('link').first().focus();
    await page.keyboard.press('Tab'); await page.keyboard.press('Enter');
    await page.waitForURL(/step=requirements/);
    assert.equal(await page.locator('a[aria-current=step]').textContent().then((s) => s.includes('Yêu cầu')), true);
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ status: 'passed', setID, artifacts: out, checks: ['create redirects', 'four steps', 'mixed file outcomes', 'preview table', 'explicit scope exclusion', 'approve/extract survives reload', 'version identity/history', 'legacy deep link', '390px layout', 'keyboard navigation', 'no browser errors'] }, null, 2));
  } catch (error) {
    await page.screenshot({ path: path.join(out, 'failure.png'), fullPage: true });
    console.error(JSON.stringify({ setID, url: page.url(), browserErrors: errors, error: error.message }, null, 2));
    process.exitCode = 1;
  } finally { await browser.close(); }
})();
