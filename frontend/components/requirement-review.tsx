"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { Requirement } from "@/lib/types";

export function RequirementReview({ requirement, canReview = false }: { requirement: Requirement; canReview?: boolean }) {
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
  const [pending, setPending] = useState(false);
  const saving = useRef(false);
  const command = useRef<{ body: string; key: string } | undefined>(undefined);
  const decide = async (decision: "APPROVED" | "REJECTED") => {
    if (saving.current) return;
    setError("");
    if (!reviewer.trim()) {
      setError("Reviewer name is required.");
      return;
    }
    saving.current = true;
    setPending(true);
    try {
      const body = JSON.stringify({ reviewer_name: reviewer.trim(), decision, comment: comment.trim(),
        expected_hash: requirement.review_hash, title, statement, actor, precondition, postcondition, risk, priority: requirement.priority });
      if (command.current?.body !== body) command.current = { body, key: crypto.randomUUID() };
      const response = await fetch(`/api/backend/api/requirements/${requirement.id}/review`, {
        method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": command.current.key }, body,
      });
      const payload = (await response.json().catch(() => ({}))) as { error?: string; requirement?: Requirement };
      if (!response.ok) {
        setError(payload.error ?? "Could not review requirement.");
        return;
      }
      command.current = undefined;
      // A new immutable revision is a new canonical document. Load it directly
      // so no prefetched route state can leave the reviewed old form visible.
      if (payload.requirement && payload.requirement.id !== requirement.id) window.location.assign(`/documents/${requirement.document_set_id}/requirements/${payload.requirement.id}`);
      else router.refresh();
    } catch {
      setError("Mất kết nối. Thử lại cùng yêu cầu không ghi trùng quyết định đã lưu.");
    } finally {
      saving.current = false;
      setPending(false);
    }
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
        {requirement.review_blockers.length ? <p className="notice">Cần xử lý điều kiện trước khi duyệt: {requirement.review_blockers.join(", ")}. Conflict/TBD phải có nội dung làm rõ; sửa title/risk không thay thế bước này.</p> : null}
        <div className="decision-actions"><button className="button secondary" type="button" disabled={pending || !canReview} onClick={() => decide("REJECTED")}>Reject</button><button className="button" type="button" disabled={pending || !canReview || requirement.review_blockers.length>0} onClick={() => decide("APPROVED")}>Accept / save edit</button></div>
      </div>
    </section>
  );
}
