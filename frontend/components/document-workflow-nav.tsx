"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type { DocumentWorkflow } from "@/lib/types";
import { documentWorkflowCopy as copy } from "@/lib/document-workflow-copy";

export type WorkspaceStep = "documents" | "requirements" | "test-cases" | "use-export";
const steps = [{ id: "documents", label: copy.steps.documents, key: "SOURCE" },
  { id: "requirements", label: copy.steps.requirements, key: "REQUIREMENTS" },
  { id: "test-cases", label: copy.steps.testCases, key: "TESTCASES" },
  { id: "use-export", label: copy.steps.useExport, key: "RELEASE" }] as const;
const labels: Record<string, string> = { READY: "Cần bạn xem", BLOCKED: "Chưa thể tiếp tục", IN_PROGRESS: "Đang xử lý", COMPLETE: "Sẵn sàng", FAILED: "Cần xử lý lỗi" };
const next: Record<string, [WorkspaceStep, string]> = {
  UPLOAD_DOCUMENT: ["documents", "Tải tài liệu để bắt đầu"], WAIT_FOR_PARSE: ["documents", "Tài liệu đang được đọc; tiến độ tự cập nhật"],
  INDEX_DOCUMENTS: ["documents", "Nguồn đang được chuẩn bị; kiểm tra file lỗi nếu có"],
  REVIEW_SOURCE: ["documents", "Xem và duyệt nguồn tài liệu"], EXTRACT_REQUIREMENTS: ["documents", "Nguồn đã duyệt; yêu cầu AI trích xuất"],
  REVIEW_REQUIREMENTS: ["requirements", "Xem và duyệt yêu cầu"], GENERATE_TESTCASES: ["test-cases", "Sinh testcase cho yêu cầu đã duyệt"],
  PUBLISH_SUITE_RELEASE: ["use-export", "Chốt bộ testcase và sử dụng kết quả"], REVIEW_WORKFLOW: ["test-cases", "Xem lại testcase và kết quả"],
  WAIT_FOR_ACTIVE_JOB: ["documents", "Đang xử lý. Bạn có thể rời trang và quay lại sau"],
};

export function DocumentWorkflowNav({ setId, workflow: initialWorkflow, active = "documents" }: {
  setId: number; workflow: DocumentWorkflow; active?: WorkspaceStep;
}) {
  const [workflow, setWorkflow] = useState(initialWorkflow);
  const heading = useRef<HTMLHeadingElement>(null);
  const previous = useRef(active);
  const navigating = useRef(false);
  useEffect(() => {
    if (previous.current !== active) heading.current?.focus(); previous.current = active;
    navigating.current = false;
  }, [active]);
  useEffect(() => {
    const controller = new AbortController();
    let busy = false;
    const refresh = async () => {
      if (busy || document.visibilityState !== "visible" || navigating.current) return;
      busy = true;
      try {
        const response = await fetch(`/api/backend/api/document-sets/${setId}/workflow`, { cache: "no-store", signal: controller.signal });
        if (!response.ok) return;
        const state = await response.json() as DocumentWorkflow;
        if (controller.signal.aborted) return;
        setWorkflow(state);
        // Inventories poll their own JSON. A read-model timestamp change must
        // never remount an editor or a release confirmation being filled in.
      } catch { /* Keep last known status; the next poll retries. */ }
      finally { busy = false; }
    };
    const timer = window.setInterval(refresh, 3000);
    void refresh();
    document.addEventListener("visibilitychange", refresh);
    window.addEventListener("document-workspace-updated", refresh);
    return () => { controller.abort(); window.clearInterval(timer); document.removeEventListener("visibilitychange", refresh); window.removeEventListener("document-workspace-updated", refresh); };
  }, [setId, active]);
  const action: [WorkspaceStep, string] = workflow.next_action === "WAIT_FOR_PARSE" && workflow.source_intents[0]?.status === "FAILED"
    ? ["documents", "Kiểm tra file lỗi; tải bản đã sửa hoặc loại rõ khỏi phạm vi để tiếp tục"]
    : next[workflow.next_action] ?? next.REVIEW_WORKFLOW;
  return <section className="workflow-navigation">
    <nav className="workflow-stepper" aria-label="Các bước từ tài liệu đến testcase">
      {steps.map((step, i) => {
        const state = workflow.steps.find((item) => item.key === step.key);
        return <Link prefetch={false} key={step.id} href={`/documents/${setId}?step=${step.id}`} onClick={() => { navigating.current = active !== step.id; }} aria-current={active === step.id ? "step" : undefined}>
          <span className="step-number" aria-hidden="true">{i + 1}</span><span><strong>{step.label}</strong>
            <small>{state?.total_units ? `${labels[state.state]} · ${state.completed_units}/${state.total_units}` : "Chưa bắt đầu"}</small></span>
        </Link>;
      })}
    </nav>
    <div className="workflow-next"><div><h2 ref={heading} tabIndex={-1}>{steps.find((step) => step.id === active)?.label}</h2>
      <p role="status">Cần làm tiếp: {action[1]}</p></div>
      <Link prefetch={false} className="button secondary" onClick={() => { navigating.current = active !== action[0]; }} href={`/documents/${setId}?step=${action[0]}#workspace-content`}>Đến bước cần làm</Link></div>
  </section>;
}
