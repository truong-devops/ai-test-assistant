"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { BaselineView } from "@/lib/types";
import { formatDate } from "@/lib/presentation";

export function BaselineSelector({ projectId, view }: { projectId: number; view: BaselineView }) {
  const router = useRouter();
  const [releaseId, setReleaseId] = useState(view.baseline ? String(view.baseline.suite_release_id) : "");
  const [mode, setMode] = useState<string>(view.baseline?.selection_mode ?? "MAPPED_WITH_FULL_FALLBACK");
  const [reviewer, setReviewer] = useState(view.baseline?.selected_by ?? "");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();

  const save = () => {
    const selected = view.candidates.find((item) => item.suite_release_id === Number(releaseId));
    if (!selected || !reviewer.trim()) {
      setError("Choose a published suite release and enter the selector name.");
      return;
    }
    setError("");
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/projects/${projectId}/document-baseline`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({
          document_set_id: selected.document_set_id,
          test_suite_id: selected.test_suite_id,
          suite_release_id: selected.suite_release_id,
          selection_mode: mode,
          selected_by: reviewer.trim(),
        }),
      });
      const payload = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        setError(payload.error ?? "Could not bind baseline.");
        return;
      }
      router.refresh();
    });
  };

  return <section className="panel side-section">
    <p className="eyebrow">Document authority</p>
    <h2>Execution baseline</h2>
    <p className="page-description">A webhook pins one immutable suite release and its exact testcase revisions before technical analysis starts.</p>
    <div className="stack" style={{ marginTop: 14 }}>
      <label><span>Published suite release</span><select value={releaseId} onChange={(event) => setReleaseId(event.target.value)}>
        <option value="">Choose release…</option>
        {view.candidates.map((item) => <option key={item.suite_release_id} value={item.suite_release_id} disabled={!item.approved_test_cases}>
          {item.document_set_name} / {item.test_suite_name} · R{item.release_number} · {formatDate(item.release_published_at)} ({item.approved_test_cases} cases)
        </option>)}
      </select></label>
      <label><span>Scope policy</span><select value={mode} onChange={(event) => setMode(event.target.value)}>
        <option value="MAPPED_WITH_FULL_FALLBACK">Explicit mapping + safe full fallback</option>
        <option value="FULL_APPROVED">Always full published release</option>
      </select></label>
      <label><span>Selected by</span><input value={reviewer} onChange={(event) => setReviewer(event.target.value)} /></label>
      <button className="button" disabled={pending} onClick={save}>{pending ? "Saving…" : "Save baseline"}</button>
      {error ? <p className="form-error">{error}</p> : null}
    </div>
  </section>;
}
