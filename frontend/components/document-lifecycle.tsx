"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { DocumentSet } from "@/lib/types";

export function DocumentLifecycle({ set }: { set: DocumentSet }) {
	const router = useRouter();
	const [retention, setRetention] = useState(String(set.retention_days || 365));
	const [actor, setActor] = useState("");
	const [reason, setReason] = useState("");
	const [error, setError] = useState("");
	const [pending, startTransition] = useTransition();
	const save = (status: "ACTIVE" | "ARCHIVED") => {
		const days = Number(retention);
		if (!actor.trim() || !reason.trim() || !Number.isInteger(days) || days < 30 || days > 3650) { setError("Actor, reason, and retention from 30 to 3650 days are required."); return; }
		startTransition(async () => {
			const response = await fetch(`/api/backend/api/document-sets/${set.id}/lifecycle`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ status, retention_days: days, actor: actor.trim(), reason: reason.trim() }) });
			const body = (await response.json().catch(() => ({}))) as { error?: string };
			if (!response.ok) setError(body.error ?? "Could not update lifecycle policy."); else { setError(""); router.refresh(); }
		});
	};
	return <details className="panel side-section"><summary>Retention and archive policy</summary><div className="stack" style={{ marginTop: 12 }}><label><span>Retention (days)</span><input type="number" min={30} max={3650} value={retention} onChange={(event) => setRetention(event.target.value)} /></label><label><span>Actor</span><input value={actor} onChange={(event) => setActor(event.target.value)} /></label><label><span>Audit reason</span><input value={reason} onChange={(event) => setReason(event.target.value)} /></label><button className="button secondary" disabled={pending} onClick={() => save(set.status === "ACTIVE" ? "ARCHIVED" : "ACTIVE")}>{set.status === "ACTIVE" ? "Archive set" : "Restore set"}</button><button className="button secondary" disabled={pending} onClick={() => save(set.status)}>Save retention</button>{error ? <p className="form-error">{error}</p> : null}<p className="table-subtitle">Archiving is recoverable and blocks new uploads. Physical deletion is intentionally not automatic.</p></div></details>;
}
