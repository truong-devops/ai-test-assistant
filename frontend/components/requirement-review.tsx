"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { Requirement } from "@/lib/types";

export function RequirementReview({ requirement }: { requirement: Requirement }) {
  const router = useRouter();
  const [reviewer, setReviewer] = useState("");
  const [comment, setComment] = useState("");
  const [title, setTitle] = useState(requirement.title);
  const [statement, setStatement] = useState(requirement.statement);
  const [actor, setActor] = useState(requirement.actor);
  const [precondition, setPrecondition] = useState(requirement.precondition);
  const [postcondition, setPostcondition] = useState(requirement.postcondition);
  const [risk, setRisk] = useState<string>(requirement.risk);
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const decide = (decision: "APPROVED" | "REJECTED") => {
    setError("");
    if (!reviewer.trim()) {
      setError("Reviewer name is required.");
      return;
    }
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/requirements/${requirement.id}/review`, {
        method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ reviewer_name: reviewer.trim(), decision, comment: comment.trim(),
          title, statement, actor, precondition, postcondition, risk, priority: requirement.priority }),
      });
      const payload = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        setError(payload.error ?? "Could not review requirement.");
        return;
      }
      router.refresh();
    });
  };
  return (
    <section className="panel workflow-review">
      <div className="panel-header"><div><h2>Human review</h2><p>Editing creates a new immutable requirement version before the decision is recorded.</p></div></div>
      <div className="panel-body">
        <div className="connect-grid">
          <label><span>Reviewer</span><input value={reviewer} onChange={(event) => setReviewer(event.target.value)} maxLength={160} /></label>
          <label><span>Risk</span><select value={risk} onChange={(event) => setRisk(event.target.value)}><option>LOW</option><option>MEDIUM</option><option>HIGH</option></select></label>
          <label className="repository-url-field"><span>Title</span><input value={title} onChange={(event) => setTitle(event.target.value)} /></label>
          <label className="repository-url-field"><span>Statement</span><textarea value={statement} onChange={(event) => setStatement(event.target.value)} /></label>
          <label><span>Actor</span><input value={actor} onChange={(event) => setActor(event.target.value)} /></label>
          <label><span>Precondition</span><input value={precondition} onChange={(event) => setPrecondition(event.target.value)} /></label>
          <label><span>Postcondition</span><input value={postcondition} onChange={(event) => setPostcondition(event.target.value)} /></label>
          <label><span>Review comment</span><input value={comment} onChange={(event) => setComment(event.target.value)} /></label>
        </div>
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        <div className="decision-actions"><button className="button secondary" type="button" disabled={pending} onClick={() => decide("REJECTED")}>Reject</button><button className="button" type="button" disabled={pending} onClick={() => decide("APPROVED")}>Accept / save edit</button></div>
      </div>
    </section>
  );
}
