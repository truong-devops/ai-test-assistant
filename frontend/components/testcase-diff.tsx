"use client";

import { useEffect, useState } from "react";
import type { TestCaseRevisionDiff } from "@/lib/types";

export const fieldNames: Record<string, string> = {
  title: "Tiêu đề",
  test_type: "Loại kiểm thử",
  risk: "Rủi ro",
  actor: "Tác nhân",
  precondition: "Tiền điều kiện",
  test_data: "Dữ liệu",
  steps: "Bước thực hiện",
  action: "Thao tác",
  expected_result: "Kết quả mong đợi",
  postcondition: "Hậu điều kiện",
  assumptions: "Giả định",
  requirement_revision_ids: "Requirement revision",
  evidence_refs: "Bằng chứng",
  source_snapshot_id: "Snapshot nguồn",
  requirement_revision_id: "Requirement revision",
  requirement_evidence_id: "Citation",
  document_version_id: "Document version",
  document_block_id: "Source block",
  source_locator: "Vị trí nguồn",
  excerpt_hash: "Mã kiểm chứng",
};
export function DiffValue({ value }: { value: unknown }) {
  if (value === undefined || value === null || value === "")
    return <span>—</span>;
  if (Array.isArray(value))
    return (
      <ol>
        {value.map((item, index) => (
          <li key={index}>
            <DiffValue value={item} />
          </li>
        ))}
      </ol>
    );
  if (typeof value === "object")
    return (
      <dl>
        {Object.entries(value)
          .filter(([key]) => key !== "excerpt_hash")
          .map(([key, item]) => (
            <div key={key}>
              <dt>{fieldNames[key] ?? key}</dt>
              <dd>
                <DiffValue value={item} />
              </dd>
            </div>
          ))}
      </dl>
    );
  return <span className="revision-value">{String(value)}</span>;
}
export function DiffRows({
  changes,
}: {
  changes: TestCaseRevisionDiff["changes"];
}) {
  return (
    <div className="table-wrap" tabIndex={0} aria-label="Bảng so sánh nội dung">
      <table className="data-table">
        <caption>Khác biệt nội dung theo thứ tự bước và citation</caption>
        <thead>
          <tr>
            <th scope="col">Trường</th>
            <th scope="col">Trước</th>
            <th scope="col">Sau</th>
          </tr>
        </thead>
        <tbody>
          {changes.map((change, index) => (
            <tr key={index}>
              <th scope="row">
                {fieldNames[change.field.split("[")[0]] ?? change.field}
                {change.field.includes("[")
                  ? ` ${change.field.slice(change.field.indexOf("["))}`
                  : ""}
              </th>
              <td>
                <DiffValue value={change.before} />
              </td>
              <td>
                <DiffValue value={change.after} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!changes.length ? <p>Không có khác biệt nội dung.</p> : null}
    </div>
  );
}
export function TestcaseDiff({
  familyID,
  from,
  to,
  onReady,
}: {
  familyID: number;
  from: number;
  to: number;
  onReady?: () => void;
}) {
  const [data, setData] = useState<
    TestCaseRevisionDiff & { next_offset?: number; total: number }
  >();
  const [error, setError] = useState("");
  const [offset, setOffset] = useState(0);
  useEffect(() => {
    setOffset(0);
  }, [from, to]);
  useEffect(() => {
    const controller = new AbortController();
    setData(undefined);
    setError("");
    fetch(
      `/api/backend/api/test-case-families/${familyID}/comparison?from=${from}&to=${to}&offset=${offset}&limit=20`,
      { signal: controller.signal, cache: "no-store" },
    )
      .then(async (response) => {
        if (!response.ok)
          throw new Error(
            "Không tải được diff; chỉ so sánh revision cùng identity.",
          );
        return response.json();
      })
      .then((value) => {
        if (controller.signal.aborted) return;
        setData(value);
        onReady?.();
      })
      .catch((caught) => {
        if (!controller.signal.aborted) setError(caught.message);
      });
    return () => controller.abort();
  }, [familyID, from, to, offset, onReady]);
  return (
    <section aria-label="So sánh revision" aria-live="polite">
      {error ? (
        <p role="alert">{error}</p>
      ) : data ? (
        <>
          <h3>
            v{data.from.version_number} → v{data.to.version_number} ·{" "}
            {data.total} thay đổi
          </h3>
          <DiffRows changes={data.changes} />
          <div className="decision-actions">
            <button
              className="button secondary"
              disabled={!offset}
              onClick={() => setOffset(Math.max(0, offset - 20))}
            >
              Diff trước
            </button>
            <button
              className="button secondary"
              disabled={data.next_offset === undefined}
              onClick={() => setOffset(data.next_offset ?? offset)}
            >
              Diff tiếp
            </button>
          </div>
        </>
      ) : (
        <p role="status">Đang tải so sánh…</p>
      )}
    </section>
  );
}
