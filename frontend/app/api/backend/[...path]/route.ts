import { NextRequest } from "next/server";

export const dynamic = "force-dynamic";

const backendOrigin = process.env.BACKEND_API_URL?.replace(/\/$/, "") ?? "http://localhost:8080";

async function proxy(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path } = await context.params;
  const incoming = new URL(request.url);
  const target = `${backendOrigin}/${path.map(encodeURIComponent).join("/")}${incoming.search}`;
  const headers = new Headers({ Accept: "application/json" });
  const contentType = request.headers.get("content-type");
  const requestID = request.headers.get("x-request-id");
  if (contentType) headers.set("content-type", contentType);
  if (requestID) headers.set("x-request-id", requestID);
  try {
    const upstream = await fetch(target, {
      method: request.method,
      headers,
      body: request.method === "GET" || request.method === "HEAD" ? undefined : await request.arrayBuffer(),
      cache: "no-store",
    });
    const responseHeaders = new Headers();
    for (const name of ["content-type", "x-request-id"]) {
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
