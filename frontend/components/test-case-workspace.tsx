"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { BusinessTestCase } from "@/lib/types";
import { StatusBadge } from "@/components/status-badge";
import { humanize } from "@/lib/presentation";

export function TestCaseWorkspace({ setId, testCases }: { setId: number; testCases: BusinessTestCase[] }) {
  const router = useRouter();
  const [selected, setSelected] = useState<number[]>([]);
  const [reviewer, setReviewer] = useState("");
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const toggle = (id: number) => setSelected((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  const review = (decision: "APPROVED" | "REJECTED") => {
    setError("");
    if (!reviewer.trim() || selected.length === 0) {
      setError("Select at least one test case and enter the reviewer name.");
      return;
    }
    startTransition(async () => {
      const response = await fetch("/api/backend/api/test-cases/bulk-review", {
        method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ test_case_ids: selected, reviewer_name: reviewer.trim(), decision, comment: comment.trim() }),
      });
      const payload = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        setError(payload.error ?? "Bulk review failed.");
        return;
      }
      setSelected([]);
      router.refresh();
    });
  };
  return (
    <section className="panel">
      <div className="panel-header"><div><h2>Business test cases</h2><p>Expected results come from approved requirement evidence, not implementation code.</p></div><span className="section-counter">{testCases.length}</span></div>
      <div className="bulk-review-bar"><label><span>Reviewer</span><input value={reviewer} onChange={(event) => setReviewer(event.target.value)} /></label><label><span>Comment</span><input value={comment} onChange={(event) => setComment(event.target.value)} /></label><button className="button secondary" disabled={pending} onClick={() => review("REJECTED")}>Reject selected</button><button className="button" disabled={pending} onClick={() => review("APPROVED")}>Approve selected</button></div>
      {error ? <p className="form-error panel-message">{error}</p> : null}
      <div className="table-wrap"><table className="data-table"><thead><tr><th><span className="sr-only">Select</span></th><th>Case</th><th>Type</th><th>Risk</th><th>Confidence</th><th>Source / automation</th><th>Status</th></tr></thead><tbody>{testCases.map((item) => <tr key={item.id}>
        <td><input type="checkbox" aria-label={`Select ${item.test_case_key}`} checked={selected.includes(item.id)} onChange={() => toggle(item.id)} /></td>
        <td><Link href={`/documents/${setId}/test-cases/${item.id}`}><span className="table-title">{item.test_case_key} · {item.title}</span><span className="table-subtitle chunk-copy">{item.expected_result}</span></Link></td>
        <td><StatusBadge status={item.test_type} /></td><td><StatusBadge status={item.risk} /></td><td>{Math.round(item.confidence * 100)}%</td><td>{humanize(item.generated_by)}<span className="table-subtitle">{humanize(item.automation_status)}</span></td><td><StatusBadge status={item.status} /></td>
      </tr>)}</tbody></table></div>
    </section>
  );
}
