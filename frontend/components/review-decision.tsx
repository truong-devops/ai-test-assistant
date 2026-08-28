"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { Review } from "@/lib/types";
import { formatDate } from "@/lib/presentation";
import { StatusBadge } from "@/components/status-badge";

export function ReviewDecision({
  generatedTestId,
  enabled,
  existing,
}: {
  generatedTestId: number;
  enabled: boolean;
  existing?: Review;
}) {
  const router = useRouter();
  const [reviewerName, setReviewerName] = useState("local-reviewer");
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();

  if (existing) {
    return (
      <section className="decision-record">
        <div>
          <p className="eyebrow">Recorded decision</p>
          <h3>{existing.reviewer_name}</h3>
          <p>{existing.comment || "No reviewer comment provided."}</p>
          <small>{formatDate(existing.created_at)}</small>
        </div>
        <StatusBadge status={existing.decision} />
      </section>
    );
  }

  const decide = (action: "accept" | "reject") => {
    setError("");
    startTransition(async () => {
      try {
        const response = await fetch(`/api/backend/api/generated-tests/${generatedTestId}/${action}`, {
          method: "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({ reviewer_name: reviewerName, comment }),
        });
        if (!response.ok) {
          const body = (await response.json().catch(() => ({}))) as { error?: string };
          throw new Error(body.error ?? "Could not save the review decision.");
        }
        router.refresh();
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Could not save the review decision.");
      }
    });
  };

  return (
    <section className="review-form">
      <div className="review-form-heading">
        <div>
          <p className="eyebrow">Human decision</p>
          <h3>{enabled ? "Make the call" : "Decision unavailable"}</h3>
        </div>
        {enabled ? <span className="review-ready">Ready for review</span> : null}
      </div>
      <p className="review-form-copy">
        {enabled
          ? "Record the final judgement for this candidate. The decision is immutable and remains visible in the audit trail."
          : "The pipeline must reach Waiting Review before a final decision can be recorded."}
      </p>
      <div className="form-grid">
        <label>
          <span>Reviewer</span>
          <input value={reviewerName} onChange={(event) => setReviewerName(event.target.value)} maxLength={128} disabled={!enabled || pending} />
        </label>
        <label className="comment-field">
          <span>Reviewer note <em>optional</em></span>
          <textarea value={comment} onChange={(event) => setComment(event.target.value)} maxLength={4000} disabled={!enabled || pending} placeholder="Explain why this test should be accepted or rejected." />
        </label>
      </div>
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      <div className="decision-actions">
        <button type="button" className="button reject" disabled={!enabled || pending} onClick={() => decide("reject")}>Reject candidate</button>
        <button type="button" className="button accept" disabled={!enabled || pending} onClick={() => decide("accept")}>{pending ? "Saving…" : "Accept candidate"}</button>
      </div>
    </section>
  );
}
