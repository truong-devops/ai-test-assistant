import "server-only";
import { readFileSync } from "node:fs";

let cachedToken: string | undefined;

function token(): string {
	if (cachedToken !== undefined) return cachedToken;
	const direct = process.env.BACKEND_API_TOKEN?.trim();
	if (direct) return (cachedToken = direct);
	const file = process.env.BACKEND_API_TOKEN_FILE?.trim();
	if (!file) return (cachedToken = "");
	try { return (cachedToken = readFileSync(file, "utf8").trim()); }
	catch { return (cachedToken = ""); }
}

export function backendAuthHeaders(): Record<string, string> {
	const value = token();
	if (!value) return {};
	return {
		Authorization: `Bearer ${value}`,
		"X-Authenticated-Role": process.env.BACKEND_API_ROLE?.trim() || "viewer",
		"X-Authenticated-Actor": process.env.BACKEND_API_ACTOR?.trim() || "frontend-service",
	};
}
