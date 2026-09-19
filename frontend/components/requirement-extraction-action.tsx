"use client";

import type { DocumentWorkflowJob } from "@/lib/types";
import { WorkflowOperationAction } from "@/components/workflow-operation-action";

export function RequirementExtractionAction({ setId, initialJob, canExtract, canRetry, canCancel }: {
  setId: string;
  initialJob?: DocumentWorkflowJob;
  canExtract: boolean;
  canRetry: boolean;
  canCancel: boolean;
}) {
  return <WorkflowOperationAction setId={setId} operation="EXTRACT_REQUIREMENTS"
    label={initialJob?.status === "FAILED" ? "Thử lại trích xuất" : "Trích xuất requirements"}
    activeLabel="Đang trích xuất…" initialJob={initialJob} disabled={!canExtract}
    canRetry={canRetry} canCancel={canCancel} />;
}
