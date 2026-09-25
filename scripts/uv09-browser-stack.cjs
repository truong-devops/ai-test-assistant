// Starts opted-in local test services with explicit backend/provider configuration.
const { spawn, spawnSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const net = require("node:net");
const root = path.resolve(__dirname, "..");
const frontend = path.join(root, "frontend");
const children = [];
const providerFixture = process.env.UV09_PROVIDER_FIXTURE === "yes";
let stopping = false;
function stop() {
  if (stopping) return;
  stopping = true;
  for (const child of children.reverse()) {
    try {
      process.kill(-child.pid, "SIGTERM");
    } catch {
      /* already exited */
    }
  }
}
process.on("SIGINT", () => {
  stop();
  process.exit(130);
});
process.on("SIGTERM", () => {
  stop();
  process.exit(143);
});
process.on("exit", stop);

function assertTestDatabase(value) {
  const url = new URL(value);
  if (
    !["postgres:", "postgresql:"].includes(url.protocol) ||
    !/^\/uv09_[a-z0-9_]+$/.test(url.pathname) ||
    !["localhost", "127.0.0.1", "postgres"].includes(url.hostname) ||
    process.env.UV09_ALLOW_TEST_WRITES !== "yes"
  ) {
    throw new Error(
      "Requires UV09_ALLOW_TEST_WRITES=yes and a local/CI database named uv09_*; production targets are refused.",
    );
  }
}
async function freePort(port) {
  await new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(port, "127.0.0.1", () => server.close(resolve));
  });
}
function start(command, args, cwd, env, log) {
  const fd = fs.openSync(log, "a", 0o600);
  const child = spawn(command, args, {
    cwd,
    env,
    detached: true,
    stdio: ["ignore", fd, fd],
  });
  fs.closeSync(fd);
  child.on("error", () => {});
  children.push(child);
  return child;
}
async function ready(url, child) {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null)
      throw new Error(`Test service exited; inspect test-results/stack logs.`);
    try {
      if ((await fetch(url, { signal: AbortSignal.timeout(2000) })).ok) return;
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 300));
  }
  throw new Error(`Test service readiness timeout: ${url}`);
}
(async () => {
  if (Number(process.versions.node.split(".")[0]) < 20)
    throw new Error("UV09 browser tests require Node.js 20 or newer.");
  assertTestDatabase(process.env.UV09_DATABASE_URL || "");
  for (const port of [
    8190,
    3190,
    3191,
    3192,
    ...(providerFixture ? [8193] : []),
  ])
    await freePort(port);
  if (!fs.existsSync(path.join(frontend, ".next/BUILD_ID")))
    throw new Error("Run npm --prefix frontend run build first.");
  const logs = path.join(frontend, "test-results/stack");
  fs.mkdirSync(logs, { recursive: true });
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "ai-test-uv09-"));
  const bin = process.env.UV09_BIN_DIR
    ? path.resolve(process.env.UV09_BIN_DIR)
    : work;
  if (!process.env.UV09_BIN_DIR) {
    for (const name of ["api", "worker"]) {
      const result = spawnSync(
        "go",
        ["build", "-o", path.join(bin, name), `./cmd/${name}`],
        { cwd: path.join(root, "backend"), stdio: "inherit" },
      );
      if (result.status !== 0) throw new Error(`Could not build test ${name}`);
    }
  }
  // Remove secret/file-based config overrides inherited from a developer shell.
  const env = Object.fromEntries(
    Object.entries(process.env).filter(
      ([key]) =>
        !/^(DATABASE_URL|API_AUTH_TOKEN|BACKEND_API_|LLM_|GEMINI_|GITHUB_|GITLAB_|EMBEDDING_|DOCUMENT_STORAGE_PATH)/.test(
          key,
        ),
    ),
  );
  const token = `uv09-${crypto.randomUUID()}`;
  Object.assign(env, {
    DATABASE_URL: process.env.UV09_DATABASE_URL,
    API_AUTH_TOKEN: token,
    APP_ENV: "development",
    HTTP_ADDR: "127.0.0.1:8190",
    DOCUMENT_STORAGE_PATH: path.join(work, "documents"),
    LLM_PROVIDER: "disabled",
    WORKER_POLL_INTERVAL: "500ms",
  });
  if (providerFixture) {
    Object.assign(env, {
      LLM_PROVIDER: "gemini",
      LLM_BASE_URL: "http://127.0.0.1:8193",
      LLM_API_KEY: "uv09-local-fake-key",
      LLM_MODEL: "uv09-local-fixture",
      LLM_REQUEST_TIMEOUT: "5s",
      WORKER_MAX_ATTEMPTS: "3",
      WORKER_RETRY_DELAY: "250ms",
    });
    const fixture = start(
      process.execPath,
      [path.join(root, "scripts/uv09-provider-fixture.cjs")],
      root,
      { ...env, UV09_TEST_TOKEN: token },
      path.join(logs, "provider-fixture.log"),
    );
    await ready("http://127.0.0.1:8193/health", fixture);
  }
  const api = start(
    path.join(bin, "api"),
    [],
    root,
    env,
    path.join(logs, "api.log"),
  );
  await ready("http://127.0.0.1:8190/ready", api);
  const worker = start(
    path.join(bin, "worker"),
    [],
    root,
    env,
    path.join(logs, "worker.log"),
  );
  for (const [role, port] of [
    ["reviewer", 3190],
    ["viewer", 3191],
    ["editor", 3192],
  ]) {
    const web = start(
      process.execPath,
      [
        path.join(frontend, "node_modules/next/dist/bin/next"),
        "start",
        "--hostname",
        "127.0.0.1",
        "--port",
        String(port),
      ],
      frontend,
      {
        ...env,
        BACKEND_API_URL: "http://127.0.0.1:8190",
        BACKEND_API_TOKEN: token,
        BACKEND_API_ROLE: role,
        BACKEND_API_ACTOR: `uv09-${role}`,
      },
      path.join(logs, `${role}.log`),
    );
    await ready(`http://127.0.0.1:${port}/documents`, web);
  }
  if (worker.exitCode !== null)
    throw new Error("Test worker exited; inspect worker.log.");
  console.log(
    `Isolated UV09 storage retained at ${work}; provider=${providerFixture ? "local protocol fixture" : "disabled"}.`,
  );
  const testEnv = {
    ...env,
    UV09_STACK_READY: "yes",
    UV09_TEST_TOKEN: token,
    UV09_REVIEWER_URL: "http://127.0.0.1:3190",
    UV09_VIEWER_URL: "http://127.0.0.1:3191",
    UV09_EDITOR_URL: "http://127.0.0.1:3192",
  };
  const tests = spawn(
    process.execPath,
    [
      require.resolve("../frontend/node_modules/@playwright/test/cli"),
      "test",
      ...(providerFixture
        ? ["provider-faults.spec.ts"]
        : ["journeys.spec.ts", "roles.spec.ts"]),
      ...process.argv.slice(2),
    ],
    { cwd: frontend, env: testEnv, detached: true, stdio: "inherit" },
  );
  children.push(tests);
  const status = await new Promise((resolve, reject) => {
    tests.once("error", reject);
    tests.once("exit", resolve);
  });
  process.exitCode = typeof status === "number" ? status : 1;
})()
  .catch((error) => {
    console.error(
      error.message.replace(
        process.env.UV09_DATABASE_URL || "<none>",
        "<test database>",
      ),
    );
    process.exitCode = 1;
  })
  .finally(stop);
