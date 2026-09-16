"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

export function PipelineModeSelector({ projectId, initialMode }: { projectId: number; initialMode: "DOCUMENT_DRIVEN" | "LEGACY" }) {
	const router = useRouter();
	const [mode, setMode] = useState(initialMode);
	const [actor, setActor] = useState("");
	const [reason, setReason] = useState("");
	const [error, setError] = useState("");
	const [pending, startTransition] = useTransition();
	const save = () => {
		if (!actor.trim() || !reason.trim()) { setError("Actor and audit reason are required."); return; }
		startTransition(async () => {
			const response = await fetch(`/api/backend/api/projects/${projectId}/pipeline-mode`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ mode, actor: actor.trim(), reason: reason.trim() }) });
			const body = (await response.json().catch(() => ({}))) as { error?: string };
			if (!response.ok) setError(body.error ?? "Could not update pipeline mode.");
			else { setError(""); router.refresh(); }
		});
	};
	return <section className="panel side-section"><p className="eyebrow">Pipeline feature flag</p><h2>{initialMode.replaceAll("_", " ")}</h2><p className="page-description">Document-driven is the default. Legacy mode skips creation of a document baseline snapshot for new webhooks.</p><div className="stack" style={{ marginTop: 14 }}><label><span>Mode</span><select value={mode} onChange={(event) => setMode(event.target.value as typeof mode)}><option value="DOCUMENT_DRIVEN">Document-driven</option><option value="LEGACY">Legacy compatibility</option></select></label><label><span>Changed by</span><input value={actor} onChange={(event) => setActor(event.target.value)} /></label><label><span>Audit reason</span><input value={reason} onChange={(event) => setReason(event.target.value)} /></label><button className="button secondary" disabled={pending || mode === initialMode} onClick={save}>{pending ? "Saving…" : "Save mode"}</button>{error ? <p className="form-error">{error}</p> : null}</div></section>;
}
