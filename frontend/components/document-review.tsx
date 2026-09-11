"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

export function DocumentReview({ versionId }: { versionId: number }) {
  const router = useRouter();
  const [reviewer, setReviewer] = useState("");
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const decide = (decision: "APPROVED" | "REJECTED") => {
    setError("");
    if (!reviewer.trim()) {
      setError("Reviewer name is required.");
      return;
    }
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/document-versions/${versionId}/review`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ reviewer_name: reviewer.trim(), decision, comment: comment.trim() }),
      });
      const payload = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        setError(payload.error ?? "Could not review document version.");
        return;
      }
      router.refresh();
    });
  };
  return (
    <section className="panel workflow-review">
      <div className="panel-header"><div><h2>Source baseline review</h2><p>Requirements can only be approved from an approved document version.</p></div></div>
      <div className="panel-body">
        <div className="connect-grid">
          <label><span>Reviewer</span><input value={reviewer} onChange={(event) => setReviewer(event.target.value)} maxLength={160} disabled={pending} /></label>
          <label><span>Comment</span><input value={comment} onChange={(event) => setComment(event.target.value)} maxLength={2000} disabled={pending} /></label>
        </div>
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        <div className="decision-actions">
          <button className="button secondary" type="button" disabled={pending} onClick={() => decide("REJECTED")}>Reject source</button>
          <button className="button" type="button" disabled={pending} onClick={() => decide("APPROVED")}>Approve source</button>
        </div>
      </div>
    </section>
  );
}
