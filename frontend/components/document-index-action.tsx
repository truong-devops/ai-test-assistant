"use client";

import { useState } from "react";
import type { DocumentIndexVersionRef, DocumentWorkflowJob } from "@/lib/types";
import { WorkflowOperationAction } from "@/components/workflow-operation-action";

type BlockedResponse = {
  code?: string;
  details?: DocumentIndexVersionRef[];
};

export function DocumentIndexAction({ setId, hasGeneration, initialJob, canIndex, canRetry, canCancel }: {
  setId: string | number;
  hasGeneration: boolean;
  initialJob?: DocumentWorkflowJob;
  canIndex: boolean;
  canRetry: boolean;
  canCancel: boolean;
}) {
  const [issues, setIssues] = useState<DocumentIndexVersionRef[]>([]);
  const [excluded, setExcluded] = useState<number[]>([]);
  const toggle = (versionID: number) => setExcluded((current) => current.includes(versionID)
    ? current.filter((item) => item !== versionID) : [...current, versionID]);
  const readyToExclude = issues.length > 0 && excluded.length === issues.length;

  return <div className="inline-action">
    <WorkflowOperationAction setId={setId} operation="INDEX_DOCUMENTS"
      label={issues.length ? "Xác nhận loại và tiếp tục" : hasGeneration ? "Cập nhật dữ liệu tìm kiếm" : "Tạo dữ liệu tìm kiếm"}
      activeLabel="Đang tạo dữ liệu tìm kiếm…" initialJob={initialJob}
      canRetry={canRetry} canCancel={canCancel}
      disabled={!canIndex || (issues.length > 0 && !readyToExclude)}
      payload={{ excluded_version_ids: excluded }}
      onBlocked={(result) => {
        const blocked = result as BlockedResponse;
        if (blocked.code === "SOURCE_SNAPSHOT_BLOCKED" && blocked.details?.length) {
          setIssues(blocked.details);
          setExcluded([]);
        }
      }} />
    {issues.length ? <div className="notice" role="group" aria-label="Tài liệu chưa thể đưa vào index">
      <strong>Có tài liệu chưa sẵn sàng.</strong>
      <p>Parse/sửa tài liệu, hoặc tích chọn toàn bộ phiên bản dưới đây để loại rõ ràng khỏi snapshot lần này:</p>
      {issues.map((issue) => <label key={issue.document_version_id} className="checkbox-row">
        <input type="checkbox" checked={excluded.includes(issue.document_version_id)}
          onChange={() => toggle(issue.document_version_id)} />
        <span>{issue.document_name} v{issue.version_number} · {issue.parse_status}</span>
      </label>)}
    </div> : null}
  </div>;
}
