"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { DocumentIndexVersionRef } from "@/lib/types";

type BlockedResponse = {
  error?: string;
  code?: string;
  issues?: DocumentIndexVersionRef[];
};

export function DocumentIndexAction({ endpoint, hasGeneration }: {
  endpoint: string;
  hasGeneration: boolean;
}) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [issues, setIssues] = useState<DocumentIndexVersionRef[]>([]);
  const [excluded, setExcluded] = useState<number[]>([]);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const run = (confirmedExclusions: number[] = []) => {
    setMessage("");
    setError("");
    startTransition(async () => {
      try {
        const response = await fetch(`/api/backend${endpoint}`, {
          method: "POST",
          headers: { Accept: "application/json", "Content-Type": "application/json" },
          body: JSON.stringify({ excluded_version_ids: confirmedExclusions }),
        });
        const payload = (await response.json().catch(() => ({}))) as BlockedResponse;
        if (!response.ok) {
          if (response.status === 409 && payload.code === "SOURCE_SNAPSHOT_BLOCKED" && payload.issues?.length) {
            setIssues(payload.issues);
            setExcluded([]);
          }
          throw new Error(payload.error ?? `Request failed (${response.status})`);
        }
        setIssues([]);
        setExcluded([]);
        setMessage("Đã tạo dữ liệu tìm kiếm cho snapshot nguồn đã chọn.");
        router.refresh();
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Không thể tạo dữ liệu tìm kiếm.");
      }
    });
  };

  const toggle = (versionID: number) => {
    setExcluded((current) => current.includes(versionID)
      ? current.filter((item) => item !== versionID)
      : [...current, versionID]);
  };

  return (
    <div className="inline-action">
      <button className="button" type="button" onClick={() => run()} disabled={pending}>
        {pending ? "Đang xử lý…" : hasGeneration ? "Cập nhật dữ liệu tìm kiếm" : "Tạo dữ liệu tìm kiếm"}
      </button>
      {issues.length ? (
        <div className="notice" role="group" aria-label="Tài liệu chưa thể đưa vào index">
          <strong>Có tài liệu chưa sẵn sàng.</strong>
          <p>Sửa/parse lại tài liệu, hoặc tích chọn để xác nhận loại khỏi snapshot lần này:</p>
          {issues.map((issue) => (
            <label key={issue.document_version_id} className="checkbox-row">
              <input
                type="checkbox"
                checked={excluded.includes(issue.document_version_id)}
                onChange={() => toggle(issue.document_version_id)}
              />
              <span>{issue.document_name} v{issue.version_number} · {issue.parse_status}</span>
            </label>
          ))}
          <button
            className="button secondary"
            type="button"
            disabled={pending || excluded.length !== issues.length}
            onClick={() => run(excluded)}
          >
            Xác nhận loại và tiếp tục
          </button>
        </div>
      ) : null}
      {message ? <small className="form-success" role="status">{message}</small> : null}
      {error ? <small className="form-error" role="alert">{error}</small> : null}
    </div>
  );
}
