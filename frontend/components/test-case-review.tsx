"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { BusinessTestCase } from "@/lib/types";

export function TestCaseReview({ testCase }: { testCase: BusinessTestCase }) {
  const router = useRouter();
  const [reviewer, setReviewer] = useState("");
  const [comment, setComment] = useState("");
  const [title, setTitle] = useState(testCase.title);
  const [precondition, setPrecondition] = useState(testCase.precondition);
  const [testData, setTestData] = useState(testCase.test_data);
  const [expected, setExpected] = useState(testCase.expected_result);
  const [postcondition, setPostcondition] = useState(testCase.postcondition);
  const [risk, setRisk] = useState<string>(testCase.risk);
  const [error, setError] = useState("");
  const [pending, startTransition] = useTransition();
  const decide = (decision: "APPROVED" | "REJECTED") => {
    if (!reviewer.trim()) {
      setError("Reviewer name is required.");
      return;
    }
    setError("");
    startTransition(async () => {
      const response = await fetch(`/api/backend/api/test-cases/${testCase.id}/review`, {
        method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ reviewer_name: reviewer.trim(), decision, comment: comment.trim(),
          title, precondition, test_data: testData, expected_result: expected, postcondition, risk,
          expected_content_hash: testCase.content_hash }),
      });
      const payload = (await response.json().catch(() => ({}))) as {
        error?: string; test_case?: BusinessTestCase;
      };
      if (!response.ok) {
        setError(payload.error ?? "Could not review test case.");
        return;
      }
      const reviewed = payload.test_case;
      if (reviewed && reviewed.id !== testCase.id) {
        router.push(`/documents/${testCase.document_set_id}/test-cases/${reviewed.id}?created=v${reviewed.version_number}`);
        return;
      }
      router.refresh();
    });
  };
  return <section className="panel workflow-review"><div className="panel-header"><div><h2>QA/Test Lead review</h2><p>Editing creates a new version while retaining requirement links and evidence.</p></div></div><div className="panel-body"><div className="connect-grid">
    <label><span>Reviewer</span><input value={reviewer} onChange={(event) => setReviewer(event.target.value)} /></label><label><span>Risk</span><select value={risk} onChange={(event) => setRisk(event.target.value)}><option>LOW</option><option>MEDIUM</option><option>HIGH</option></select></label>
    <label className="repository-url-field"><span>Title</span><input value={title} onChange={(event) => setTitle(event.target.value)} /></label><label className="repository-url-field"><span>Expected result</span><textarea value={expected} onChange={(event) => setExpected(event.target.value)} /></label>
    <label><span>Precondition</span><input value={precondition} onChange={(event) => setPrecondition(event.target.value)} /></label><label><span>Test data</span><input value={testData} onChange={(event) => setTestData(event.target.value)} /></label><label><span>Postcondition</span><input value={postcondition} onChange={(event) => setPostcondition(event.target.value)} /></label><label><span>Comment</span><input value={comment} onChange={(event) => setComment(event.target.value)} /></label>
  </div>{error ? <p className="form-error">{error}</p> : null}<div className="decision-actions"><button className="button secondary" type="button" disabled={pending} onClick={() => decide("REJECTED")}>Reject</button><button className="button" type="button" disabled={pending} onClick={() => decide("APPROVED")}>Accept / save edit</button></div></div></section>;
}
