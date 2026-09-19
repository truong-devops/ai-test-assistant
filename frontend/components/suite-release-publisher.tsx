"use client";

import { useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { BusinessTestCase, SuiteRelease } from "@/lib/types";
import { formatDate } from "@/lib/presentation";

export function SuiteReleasePublisher({ setId, suiteId, testCases, releases }: {
  setId: number;
  suiteId: number;
  testCases: BusinessTestCase[];
  releases: SuiteRelease[];
}) {
  const router = useRouter();
  const approved = testCases.filter((item) => item.status === "APPROVED");
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [publishedBy, setPublishedBy] = useState("");
  const [scopeDecision, setScopeDecision] = useState("");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const command = useRef<{ payload: string; key: string } | undefined>(undefined);

  const toggle = (id: number) => setSelected((current) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });

  const publish = () => {
    setError("");
    const revisions = approved.filter((item) => selected.has(item.id));
    if (!revisions.length || !publishedBy.trim()) {
      setError("Select at least one approved revision and enter the publisher name.");
      return;
    }
    const snapshots = new Set(revisions.map((item) => item.source_snapshot_id).filter(Boolean));
    if (snapshots.size !== 1) {
      setError("Selected revisions must use the same reviewed source snapshot.");
      return;
    }
    const body = {
      test_suite_id: suiteId,
      source_snapshot_id: [...snapshots][0],
      revision_ids: revisions.map((item) => item.id),
      published_by: publishedBy.trim(),
      scope_decision: scopeDecision.trim(),
    };
    const payload = JSON.stringify(body);
    if (!command.current || command.current.payload !== payload) {
      command.current = { payload, key: crypto.randomUUID() };
    }
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/document-sets/${setId}/suite-releases`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json",
          "Idempotency-Key": command.current?.key ?? crypto.randomUUID() },
        body: payload,
      });
      const result = (await response.json().catch(() => ({}))) as SuiteRelease & { error?: string };
      if (!response.ok) {
        setError(result.error ?? "Could not publish the suite release.");
        return;
      }
      setSelected(new Set());
      command.current = undefined;
      router.refresh();
    });
  };

  return <section className="panel">
    <div className="panel-header"><div><h2>Publish an immutable suite release</h2><p>Select exact approved revisions. A later draft or approval will not change an existing release, project baseline or run.</p></div><span className="section-counter">{releases.length} releases</span></div>
    <div className="bulk-review-bar">
      <span><strong>{selected.size}</strong> of {approved.length} approved revisions selected</span>
      <button className="button secondary" type="button" disabled={!approved.length || pending} onClick={() => setSelected(new Set(approved.map((item) => item.id)))}>Select all approved</button>
      <button className="button ghost" type="button" disabled={!selected.size || pending} onClick={() => setSelected(new Set())}>Clear</button>
      <label><span>Published by</span><input value={publishedBy} onChange={(event) => setPublishedBy(event.target.value)} placeholder="QA / Test Lead" /></label>
      <label><span>Partial-scope decision</span><input value={scopeDecision} onChange={(event) => setScopeDecision(event.target.value)} placeholder="Required only when approved requirements are omitted" /></label>
      <button className="button" type="button" disabled={pending || !selected.size} onClick={publish}>{pending ? "Publishing…" : "Publish next release"}</button>
    </div>
    {error ? <p className="form-error panel-message">{error}</p> : null}
    <div className="table-wrap"><table className="data-table"><thead><tr><th>Select</th><th>Approved revision</th><th>Source snapshot</th><th>Expected result</th></tr></thead><tbody>{approved.map((item) => <tr key={item.id}><td><input type="checkbox" checked={selected.has(item.id)} disabled={pending} onChange={() => toggle(item.id)} aria-label={`Select ${item.test_case_key} revision ${item.version_number}`} /></td><td><span className="table-title">{item.test_case_key} v{item.version_number}</span><span className="table-subtitle">{item.title}</span></td><td>{item.source_snapshot_id ? `#${item.source_snapshot_id}` : "Missing"}</td><td>{item.expected_result}</td></tr>)}</tbody></table></div>
    {releases.length ? <div className="evidence-list">{releases.map((release) => <p key={release.id}><strong>R{release.release_number}</strong> · {release.items.length} revisions · {release.scope_status.toLowerCase()} · {formatDate(release.published_at)}<span className="table-subtitle">Manifest <span className="mono">{release.manifest_hash.slice(0, 16)}…</span> · by {release.published_by}</span></p>)}</div> : <p className="table-subtitle">No published release yet. Projects cannot bind a mutable working set.</p>}
  </section>;
}
