"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { DocumentWorkflowJob, DocumentWorkflowOperation, WorkflowBlockingReason } from "@/lib/types";
import { useWorkflowJob } from "@/lib/use-workflow-job";

type ErrorEnvelope = {
  error?: string;
  message?: string;
  code?: string;
  retryable?: boolean;
  blocked_by?: WorkflowBlockingReason[];
  next_action?: string;
};

export function WorkflowOperationAction({ setId, operation, label, activeLabel,
  initialJob, disabled = false, canRetry = false, canCancel = false,
  payload = {}, onBlocked }: {
  setId: string | number;
  operation: DocumentWorkflowOperation;
  label: string;
  activeLabel: string;
  initialJob?: DocumentWorkflowJob;
  disabled?: boolean;
  canRetry?: boolean;
  canCancel?: boolean;
  payload?: Record<string, unknown>;
  onBlocked?: (error: ErrorEnvelope) => void;
}) {
  const router = useRouter();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const command = useRef<{ signature: string; key: string } | undefined>(undefined);
  const { job, setJob, pollError, active } = useWorkflowJob(initialJob, (finished) => {
    if (finished.status === "SUCCEEDED") {
      setError("");
      router.refresh();
    } else if (finished.status === "FAILED" || finished.status === "PARTIAL_FAILED") {
      setError(finished.error_message || "Workflow failed.");
    }
  });

  const request = async () => {
    setPending(true);
    setError("");
    const body = JSON.stringify({ operation, ...payload });
    if (!command.current || command.current.signature !== body) {
      command.current = { signature: body, key: crypto.randomUUID() };
    }
    try {
      const response = await fetch(`/api/backend/api/document-sets/${setId}/workflow-operations`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json",
          "Idempotency-Key": command.current.key },
        body,
      });
      const result = (await response.json().catch(() => ({}))) as ErrorEnvelope & { job?: DocumentWorkflowJob };
      if (!response.ok || !result.job) {
        onBlocked?.(result);
        throw new Error(result.message ?? result.error ?? `Request failed (${response.status})`);
      }
      setJob(result.job);
      command.current = undefined;
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not reach the workflow API.");
    } finally {
      setPending(false);
    }
  };

  const mutate = async (action: "retry" | "cancel") => {
    if (!job) return;
    setPending(true);
    setError("");
    try {
      const response = await fetch(`/api/backend/api/document-workflow-jobs/${job.id}/${action}`, {
        method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ expected_revision: job.revision }),
      });
      const result = (await response.json().catch(() => ({}))) as ErrorEnvelope & { job?: DocumentWorkflowJob };
      if (!response.ok || !result.job) throw new Error(result.message ?? result.error ?? `${action} failed (${response.status})`);
      setJob(result.job);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : `Could not ${action} workflow.`);
    } finally {
      setPending(false);
    }
  };

  const percent = job?.total_units ? Math.min(100, Math.round(job.completed_units * 100 / job.total_units)) : 0;
  return <div className="inline-action">
    <button className="button" type="button" onClick={() => void request()}
      disabled={disabled || pending || active}>{pending ? "Đang gửi…" : active ? activeLabel : label}</button>
    {active && job ? <small className="table-subtitle" role="status">
      {job.status} · {job.completed_units}/{job.total_units || "?"} units{job.total_units ? ` (${percent}%)` : " · đang chuẩn bị"} · attempt {job.attempt_count}/{job.max_attempts}
    </small> : null}
    {active && job?.cancel_requested_at ? <small className="table-subtitle" role="status">
      Đã yêu cầu dừng; worker đang kết thúc attempt hiện tại an toàn.
    </small> : null}
    {active && job && canCancel && !job.cancel_requested_at ? <button className="button secondary" type="button" disabled={pending}
      onClick={() => void mutate("cancel")}>Dừng xử lý</button> : null}
    {canRetry && job?.retryable && (job.status === "FAILED" || job.status === "PARTIAL_FAILED") ?
      <button className="button secondary" type="button" disabled={pending}
        onClick={() => void mutate("retry")}>Thử lại bước lỗi</button> : null}
    {job?.status === "SUCCEEDED" ? <small className="form-success" role="status">
      Hoàn tất · {job.completed_units}/{job.total_units} units
    </small> : null}
    {error || pollError || (job?.status === "FAILED" ? job.error_message : "") ?
      <div><p className="form-error" role="alert">{pollError ? "Chưa cập nhật được tiến độ. Kiểm tra kết nối; tác vụ trên server vẫn được giữ." : "Chưa hoàn tất thao tác. Kiểm tra điều kiện nguồn/quyền và ngân sách; dùng Thử lại bước lỗi khi có."}</p>
        <details><summary>Chi tiết lỗi xử lý</summary><pre className="source-text">{error || pollError || job?.error_message}</pre></details></div> : null}
  </div>;
}
