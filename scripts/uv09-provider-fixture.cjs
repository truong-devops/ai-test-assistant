// Local Gemini protocol fixture. Never forwards requests or uses real credentials.
const http = require("node:http");
const controlToken = process.env.UV09_TEST_TOKEN;
if (!controlToken || process.env.UV09_PROVIDER_FIXTURE !== "yes")
  throw new Error("Only the opted-in UV09 runner may start this fixture");
let mode = "reject400";
let calls = [];
const modes = new Set([
  "reject400",
  "timeout",
  "invalid_enum",
  "valid",
  "hold",
]);
let pending = [];
function json(response, status, data) {
  if (response.destroyed) return;
  response.writeHead(status, { "Content-Type": "application/json" });
  response.end(JSON.stringify(data));
}
const server = http.createServer(async (request, response) => {
  if (request.url === "/health") return json(response, 200, { ready: true });
  if (request.url === "/control") {
    if (request.headers.authorization !== `Bearer ${controlToken}`)
      return json(response, 403, { error: "forbidden" });
    if (request.method === "GET") return json(response, 200, { mode, calls });
    if (request.method !== "POST")
      return json(response, 405, { error: "method" });
    const chunks = [];
    let length = 0;
    for await (const chunk of request) {
      length += chunk.length;
      if (length > 1024) return json(response, 413, { error: "too large" });
      chunks.push(chunk);
    }
    try {
      const body = JSON.parse(Buffer.concat(chunks).toString());
      if (!modes.has(body.mode)) throw new Error("mode");
      mode = body.mode;
      if (body.reset) calls = [];
      if (mode === "valid") {
        const held = pending;
        pending = [];
        for (const finish of held) finish();
      }
      return json(response, 200, { mode, calls });
    } catch {
      return json(response, 400, { error: "invalid control" });
    }
  }
  if (request.url !== "/interactions" || request.method !== "POST")
    return json(response, 404, { error: "not found" });
  if (request.headers["x-goog-api-key"] !== "uv09-local-fake-key")
    return json(response, 403, { error: "fixture key required" });
  // Parse only to select the response schema. Do not persist prompt/header data.
  const chunks = [];
  let length = 0;
  for await (const chunk of request) {
    length += chunk.length;
    if (length > 1024 * 1024)
      return json(response, 413, { error: "fixture input too large" });
    chunks.push(chunk);
  }
  let input;
  try {
    input = JSON.parse(Buffer.concat(chunks).toString());
  } catch {
    return json(response, 400, { error: "invalid fixture input" });
  }
  const generation = !!input.response_format?.schema?.properties?.test_cases;
  const call = { mode, ordinal: calls.length + 1, closed: false };
  calls.push(call);
  response.once("close", () => {
    call.closed = true;
  });
  if (mode === "reject400")
    return json(response, 400, {
      error: {
        code: "UV09_PERMANENT",
        message: "UV09 fixture rejected request",
      },
    });
  if (mode === "timeout") {
    // Longer than the test client's 5s deadline. No upstream request exists.
    setTimeout(
      () => json(response, 504, { error: "fixture delayed response" }),
      10000,
    );
    return;
  }
  const requirement = {
    identifier: "REQ-FAULT",
    title: "Create order",
    statement:
      "The system must create an order when a customer confirms checkout.",
    requirement_type: "FUNCTIONAL",
    flow_type: "NONE",
    actor: "Customer",
    precondition: "",
    postcondition: "",
    priority: "HIGH",
    risk: "LOW",
    status: mode === "invalid_enum" ? "APPROVED" : "DRAFT",
    confidence: 0.9,
    assumptions: [],
    steps: [],
  };
  const testCase = {
    title: "Create order",
    test_type: mode === "invalid_enum" ? "INVALID_TYPE" : "HAPPY",
    risk: "LOW",
    actor: "Customer",
    precondition: "",
    test_data: "",
    expected_result: requirement.statement,
    postcondition: "",
    automation_status: "MANUAL",
    confidence: 0.9,
    assumptions: [],
    steps: [
      { action: "Confirm checkout", expected_result: requirement.statement },
    ],
  };
  const finish = () =>
    json(response, 200, {
      id: `uv09-fixture-${call.ordinal}`,
      model: "uv09-local-fixture",
      status: "completed",
      steps: [
        {
          type: "model_output",
          content: [
            {
              type: "text",
              text: JSON.stringify(
                generation
                  ? { test_cases: [testCase] }
                  : { requirements: [requirement] },
              ),
            },
          ],
        },
      ],
      usage: {
        total_input_tokens: 30,
        total_output_tokens: 10,
        total_tokens: 40,
      },
    });
  if (mode === "hold") pending.push(finish);
  else finish();
});
server.listen(8193, "127.0.0.1");
