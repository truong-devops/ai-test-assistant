import { NextRequest } from "next/server";
import { backendAuthHeaders } from "@/lib/backend-auth";

export const dynamic = "force-dynamic";

const backendOrigin = process.env.BACKEND_API_URL?.replace(/\/$/, "") ?? "http://localhost:8080";
const configuredDocumentMaxBytes = Number(process.env.DOCUMENT_MAX_UPLOAD_BYTES ?? 16 * 1024 * 1024);
const documentMaxBytes = Number.isSafeInteger(configuredDocumentMaxBytes) &&
  configuredDocumentMaxBytes > 0 && configuredDocumentMaxBytes <= 256 * 1024 * 1024
  ? configuredDocumentMaxBytes
  : 16 * 1024 * 1024;
const genericMaxBodyBytes = 2 * 1024 * 1024;

class ProxyBodyTooLarge extends Error {}

function isDocumentUpload(path: string[]): boolean {
  return path[0] === "api" && path[1] === "document-sets" && path[3] === "documents" &&
    (path.length === 4 || (path.length === 6 && path[5] === "versions"));
}

async function boundedRequestBody(request: NextRequest, maxBytes: number): Promise<ArrayBuffer | undefined> {
  if (request.method === "GET" || request.method === "HEAD" || request.body === null) return undefined;
  const declaredLength = Number(request.headers.get("content-length"));
  if (Number.isFinite(declaredLength) && declaredLength > maxBytes) throw new ProxyBodyTooLarge();

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > maxBytes) {
      await reader.cancel();
      throw new ProxyBodyTooLarge();
    }
    chunks.push(value);
  }
  const buffer = new ArrayBuffer(total);
  const body = new Uint8Array(buffer);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return buffer;
}

async function proxy(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path } = await context.params;
  const incoming = new URL(request.url);
  const target = `${backendOrigin}/${path.map(encodeURIComponent).join("/")}${incoming.search}`;
  const headers = new Headers({ Accept: "application/json", ...backendAuthHeaders() });
  const contentType = request.headers.get("content-type");
  const requestID = request.headers.get("x-request-id");
  const idempotencyKey = request.headers.get("idempotency-key");
  const ifMatch = request.headers.get("if-match");
  const ifNoneMatch = request.headers.get("if-none-match");
  const ifUnmodifiedSince = request.headers.get("if-unmodified-since");
  const expectedRevision = request.headers.get("x-expected-revision");
  if (contentType) headers.set("content-type", contentType);
  if (requestID) headers.set("x-request-id", requestID);
  if (idempotencyKey) headers.set("idempotency-key", idempotencyKey);
  if (ifMatch) headers.set("if-match", ifMatch);
  if (ifNoneMatch) headers.set("if-none-match", ifNoneMatch);
  if (ifUnmodifiedSince) headers.set("if-unmodified-since", ifUnmodifiedSince);
  if (expectedRevision) headers.set("x-expected-revision", expectedRevision);
  let body: ArrayBuffer | undefined;
  try {
    const limit = isDocumentUpload(path) ? documentMaxBytes + (1 * 1024 * 1024) : genericMaxBodyBytes;
    body = await boundedRequestBody(request, limit);
  } catch (error) {
    if (error instanceof ProxyBodyTooLarge) {
      return Response.json({ error: "request body is too large" }, { status: 413 });
    }
    return Response.json({ error: "could not read request body" }, { status: 400 });
  }
  try {
    const upstream = await fetch(target, {
      method: request.method,
      headers,
      body,
      cache: "no-store",
    });
    const responseHeaders = new Headers();
    for (const name of ["content-type", "content-disposition", "content-length", "x-content-sha256",
      "x-request-id", "location", "deprecation", "link", "retry-after", "etag", "last-modified"]) {
      const value = upstream.headers.get(name);
      if (value) responseHeaders.set(name, value);
    }
    return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
  } catch {
    return Response.json({ error: "backend API is unavailable" }, { status: 502 });
  }
}

export const GET = proxy;
export const POST = proxy;
