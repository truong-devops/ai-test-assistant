import { test, expect, type APIRequestContext } from "@playwright/test";
import crypto from "node:crypto";

test("viewer/editor/reviewer policy and server-token boundary", async ({
  page,
  request,
}, info) => {
  test.skip(!process.env.UV09_STACK_READY, "Use isolated test stack.");
  const reviewer = process.env.UV09_REVIEWER_URL!;
  const token = process.env.UV09_TEST_TOKEN!;
  const api = async (
    client: APIRequestContext,
    base: string,
    route: string,
    data?: unknown,
  ) => {
    const url = `${base}/api/backend/api/${route}`;
    const response =
      data === undefined
        ? await client.get(url)
        : await client.post(url, {
            data,
            headers: { "Idempotency-Key": crypto.randomUUID() },
          });
    expect(response.ok(), `${route}: ${response.status()}`).toBeTruthy();
    expect(JSON.stringify(response.headers())).not.toContain(token);
    expect(await response.text()).not.toContain(token);
    return response.json();
  };
  const set = await api(request, reviewer, "document-sets", {
    name: `UV09 roles ${Date.now()}`,
  });
  const id = set.id;
  const spoof = {
    "X-Authenticated-Role": "admin",
    "X-Authenticated-Actor": "spoof",
    Authorization: "Bearer attacker-value",
  };
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const role of ["viewer", "editor", "reviewer"]) {
    const base = process.env[`UV09_${role.toUpperCase()}_URL`]!;
    const workflow = await api(request, base, `document-sets/${id}/workflow`);
    expect(workflow.capabilities.can_upload).toBe(role !== "viewer");
    expect(workflow.capabilities.can_review).toBe(role === "reviewer");
    await page.goto(`${base}/documents/${id}?step=test-cases`);
    await expect(
      page.getByRole("heading", { name: "Sinh và đối chiếu đề xuất" }),
    ).toBeVisible();
    expect(await page.content()).not.toContain(token);
    expect(
      await page.evaluate(() =>
        JSON.stringify({
          local: { ...localStorage },
          session: { ...sessionStorage },
          cookies: document.cookie,
        }),
      ),
    ).not.toContain(token);
    if (role !== "reviewer") {
      for (const route of [
        `document-sets/${id}/source-review`,
        `document-sets/${id}/requirement-review/bulk-review`,
        `document-sets/${id}/suite-releases`,
        "test-case-proposals/1/review",
      ]) {
        const response = await request.post(
          `${base}/api/backend/api/${route}`,
          {
            data: route.endsWith("source-review")
              ? { command: "APPROVE_AND_EXTRACT" }
              : {},
            headers: { ...spoof, "Idempotency-Key": crypto.randomUUID() },
          },
        );
        expect(response.status(), `${role} cannot elevate via ${route}`).toBe(
          403,
        );
        expect(await response.text()).not.toContain(token);
      }
      await page.goto(`${base}/documents/${id}/requirements`);
      await expect(
        page.getByRole("button", { name: "Chọn các mục trên trang này" }),
      ).toBeDisabled();
      await expect(
        page.getByText("Chế độ xem. Cần quyền reviewer để lưu quyết định."),
      ).toBeVisible();
    }
    await page.screenshot({
      path: info.outputPath(`${role}.png`),
      fullPage: true,
    });
    const missing = await request.get(
      `${base}/api/backend/api/test-exports/999999999/download`,
    );
    expect(missing.status()).toBe(404);
    expect(await missing.text()).not.toContain(token);
    expect(JSON.stringify(missing.headers())).not.toContain(token);
  }
  const viewer = process.env.UV09_VIEWER_URL!;
  const blocked = await request.post(
    `${viewer}/api/backend/api/document-sets`,
    { data: { name: "must not exist" }, headers: spoof },
  );
  expect(blocked.status()).toBe(403);
  const editor = process.env.UV09_EDITOR_URL!;
  const uploaded = await request.post(
    `${editor}/api/backend/api/document-sets/${id}/documents`,
    {
      multipart: {
        new_document: "true",
        file: {
          name: "roles.md",
          mimeType: "text/markdown",
          buffer: Buffer.from(
            "# Role check\n\nThe system must reject invalid input.\n",
          ),
        },
      },
      headers: spoof,
    },
  );
  expect(uploaded.status()).toBe(202);
  // Static browser bundles must not contain the server service token either.
  await page.goto(`${reviewer}/documents/${id}`);
  for (const source of await page
    .locator("script[src]")
    .evaluateAll((scripts) =>
      scripts.map((script) => (script as HTMLScriptElement).src),
    )) {
    const response = await request.get(source);
    expect(response.ok()).toBeTruthy();
    expect(await response.text()).not.toContain(token);
  }
  expect(errors).toEqual([]);
});
