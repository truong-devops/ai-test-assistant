"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { BaselineView } from "@/lib/types";

export function BaselineSelector({ projectId, view }: { projectId: number; view: BaselineView }) {
  const router = useRouter();
  const [candidate, setCandidate] = useState(view.baseline ? `${view.baseline.document_set_id}:${view.baseline.test_suite_id}` : "");
  const [mode, setMode] = useState<string>(view.baseline?.selection_mode ?? "MAPPED_WITH_FULL_FALLBACK");
  const [reviewer, setReviewer] = useState(view.baseline?.selected_by ?? "");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const save = () => { const [documentSetId, testSuiteId] = candidate.split(":").map(Number); if (!documentSetId || !testSuiteId || !reviewer.trim()) { setError("Choose an approved suite and enter the selector name."); return; } setError(""); startTransition(async () => { const response = await fetch(`/api/backend/api/projects/${projectId}/document-baseline`, { method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify({ document_set_id: documentSetId, test_suite_id: testSuiteId, selection_mode: mode, selected_by: reviewer.trim() }) }); const payload = (await response.json().catch(() => ({}))) as { error?: string }; if (!response.ok) { setError(payload.error ?? "Could not bind baseline."); return; } router.refresh(); }); };
  return <section className="panel side-section"><p className="eyebrow">Document authority</p><h2>Execution baseline</h2><p className="page-description">A webhook snapshots this approved suite before technical analysis starts.</p><div className="stack" style={{ marginTop: 14 }}><label><span>Document set / suite</span><select value={candidate} onChange={(event) => setCandidate(event.target.value)}><option value="">Choose baseline…</option>{view.candidates.map((item) => <option key={`${item.document_set_id}:${item.test_suite_id}`} value={`${item.document_set_id}:${item.test_suite_id}`} disabled={!item.approved_test_cases}>{item.document_set_name} / {item.test_suite_name} ({item.approved_test_cases} approved)</option>)}</select></label><label><span>Scope policy</span><select value={mode} onChange={(event) => setMode(event.target.value)}><option value="MAPPED_WITH_FULL_FALLBACK">Explicit mapping + safe full fallback</option><option value="FULL_APPROVED">Always full approved suite</option></select></label><label><span>Selected by</span><input value={reviewer} onChange={(event) => setReviewer(event.target.value)} /></label><button className="button" disabled={pending} onClick={save}>{pending ? "Saving…" : "Save baseline"}</button>{error ? <p className="form-error">{error}</p> : null}</div></section>;
}
