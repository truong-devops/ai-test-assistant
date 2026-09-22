"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import type {
  BusinessTestCase,
  BusinessTestCaseDetail,
  TestCaseFamily,
  AutomationHistory as Automation,
} from "@/lib/types";
import { AutomationHistory } from "@/components/automation-history";
import { DiffRows, TestcaseDiff, fieldNames } from "@/components/testcase-diff";
import { StatusBadge } from "@/components/status-badge";

const editFields = [
  "title",
  "actor",
  "precondition",
  "test_data",
  "expected_result",
  "postcondition",
  "risk",
  "test_type",
] as const;
function draftOf(detail: BusinessTestCaseDetail) {
  return {
    ...(Object.fromEntries(
      editFields.map((key) => [key, detail.test_case[key]]),
    ) as Record<(typeof editFields)[number], string>),
    steps: detail.steps.map(({ action, expected_result }) => ({
      action,
      expected_result,
    })),
  };
}
type History = {
  entries: {
    revision: BusinessTestCase;
    releases: { id: number; number: number }[];
  }[];
  next_before?: number;
};
type Run = {
  id: number;
  test_case_id: number;
  version: number;
  run_id: number;
  artifact_id?: number;
  status: string;
  expected: string;
  actual: string;
};
async function load<T>(url: string): Promise<T> {
  const response = await fetch(`/api/backend/api/${url}`, {
    cache: "no-store",
  });
  if (!response.ok) throw new Error("Không tải được dữ liệu. Thử lại.");
  return response.json();
}

export function TestcaseRevisionWorkspace({
  detail: initial,
  family: initialFamily,
  automation,
  canEdit,
  canReview,
}: {
  detail: BusinessTestCaseDetail;
  family: TestCaseFamily;
  automation: Automation;
  canEdit: boolean;
  canReview: boolean;
}) {
  const [detail, setDetail] = useState(initial);
  const item = detail.test_case;
  const [family, setFamily] = useState(initialFamily);
  const [base, setBase] = useState(initial);
  const [draft, setDraft] = useState(() => draftOf(initial));
  const [reason, setReason] = useState("");
  const [reviewer, setReviewer] = useState("");
  const [tab, setTab] = useState("edit");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [pending, setPending] = useState(false);
  const busy = useRef(false);
  const dirty = JSON.stringify(draft) !== JSON.stringify(draftOf(base));
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const [conflict, setConflict] = useState<BusinessTestCaseDetail>();
  const [history, setHistory] = useState<History>({ entries: [] });
  const [historyBefore, setHistoryBefore] = useState(0);
  const [from, setFrom] = useState(item.parent_revision_id ?? item.id);
  const [to, setTo] = useState(item.id);
  const [restoreReady, setRestoreReady] = useState(false);
  const diffReady = useCallback(() => setRestoreReady(true), []);
  const [scope, setScope] = useState("revision");
  const [runs, setRuns] = useState<Run[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState("");
  const [runBefore, setRunBefore] = useState(0);
  const [nextRun, setNextRun] = useState<number>();
  const command = useRef<{ body: string; key: string } | undefined>(undefined);
  useEffect(() => {
    const unload = (event: BeforeUnloadEvent) => {
      if (dirtyRef.current) {
        event.preventDefault();
      }
    };
    const leave = (event: MouseEvent) => {
      const target = event.target as Element;
      if (
        dirtyRef.current &&
        target.closest("a[href]") &&
        !window.confirm("Bạn có thay đổi chưa lưu. Rời trang và bỏ thay đổi?")
      ) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    window.addEventListener("beforeunload", unload);
    document.addEventListener("click", leave, true);
    return () => {
      window.removeEventListener("beforeunload", unload);
      document.removeEventListener("click", leave, true);
    };
  }, []);
  useEffect(() => {
    let active = true;
    load<History>(
      `test-case-families/${family.id}/history?before=${historyBefore}&limit=20`,
    )
      .then((value) => {
        if (active) setHistory(value);
      })
      .catch((caught) => {
        if (active) setError(caught.message);
      });
    return () => {
      active = false;
    };
  }, [family.id, historyBefore, item.status]);
  useEffect(() => {
    if (tab !== "runs") return;
    let active = true;
    setRunsLoading(true);
    setRunsError("");
    load<{ runs: Run[]; next_before?: number }>(
      `test-cases/${item.id}/runs?scope=${scope}&before=${runBefore}`,
    )
      .then((value) => {
        if (active) {
          setRuns(value.runs);
          setNextRun(value.next_before);
        }
      })
      .catch((caught) => {
        if (active) setRunsError(caught.message);
      })
      .finally(() => {
        if (active) setRunsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [item.id, tab, scope, runBefore]);
  const navigate = (id: number) => {
    dirtyRef.current = false;
    window.location.assign(
      `/documents/${item.document_set_id}/test-cases/${id}`,
    );
  };
  const mutate = async (
    action: "save" | "restore" | "APPROVED" | "REJECTED" | "archive",
  ) => {
    if (busy.current) return;
    if (
      (action === "save" || action === "restore" || action === "archive") &&
      !reason.trim()
    ) {
      setError("Nhập lý do thay đổi/phục hồi/lưu trữ.");
      return;
    }
    if (
      (action === "APPROVED" || action === "REJECTED") &&
      (!reviewer.trim() || dirty)
    ) {
      setError("Lưu bản nháp trước khi duyệt; nhập tên người duyệt.");
      return;
    }
    busy.current = true;
    setPending(true);
    setError("");
    setMessage("");
    const root = `test-case-families/${family.id}`;
    let url = "";
    let data: object;
    if (action === "save") {
      url = `${root}/versions`;
      data = {
        base_revision_id: base.test_case.id,
        expected_head_revision_id: family.head_revision_id,
        expected_head_token: family.head_token,
        patch: draft,
        reason,
      };
    } else if (action === "restore") {
      url = `${root}/restore`;
      data = {
        from_revision_id: to,
        expected_head_revision_id: family.head_revision_id,
        expected_head_token: family.head_token,
        reason,
      };
    } else if (action === "archive") {
      url = `${root}/archive`;
      data = { archived: !family.archived, reason };
    } else {
      url = `test-cases/${item.id}/review`;
      data = {
        decision: action,
        reviewer_name: reviewer,
        comment: reason,
        expected_content_hash: item.content_hash,
      };
    }
    const body = JSON.stringify(data);
    if (command.current?.body !== url + body)
      command.current = { body: url + body, key: crypto.randomUUID() };
    try {
      const response = await fetch(`/api/backend/api/${url}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": command.current.key,
        },
        body,
      });
      const result = await response.json();
      if (!response.ok) {
        if (result.code === "TESTCASE_HEAD_CONFLICT" && action === "save") {
          const current = await load<BusinessTestCaseDetail>(
            `test-cases/${result.current_revision_id}`,
          );
          setConflict(current);
          setFamily(await load<TestCaseFamily>(root));
        }
        throw new Error(
          `${result.field ? `${fieldNames[result.field] ?? result.field}: ` : ""}${result.error ?? "Chưa lưu được"}`,
        );
      }
      command.current = undefined;
      if (action === "save" || action === "restore") {
        navigate(result.revision.id);
        return;
      }
      if (action === "archive") {
        setFamily(result);
        setMessage(
          result.archived
            ? "Đã lưu trữ identity; lịch sử/release/run vẫn giữ nguyên."
            : "Đã mở lại identity.",
        );
      } else {
        setDetail(result);
        setFamily(await load<TestCaseFamily>(root));
        setMessage(
          `Đã lưu quyết định cho v${item.version_number} (#${item.id}).`,
        );
      }
      window.dispatchEvent(new Event("document-workspace-updated"));
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  const exportRevision = async () => {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const response = await fetch(
        `/api/backend/api/document-sets/${item.document_set_id}/exports`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            test_suite_id: item.test_suite_id,
            test_case_ids: [item.id],
            format: "MARKDOWN",
            generated_by: reviewer.trim() || "QA",
          }),
        },
      );
      const value = await response.json();
      if (!response.ok) throw new Error(value.error);
      window.location.assign(`/api/backend${value.download_url}`);
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  return (
    <div className="revision-workspace">
      <section className="panel panel-body">
        <h2>
          {item.test_case_key} · testcase v{item.version_number} · #{item.id}
        </h2>
        <p>
          Latest: v{family.latest_revision?.version_number} · Approved:{" "}
          {family.latest_approved_revision
            ? `v${family.latest_approved_revision.version_number}`
            : "chưa có"}{" "}
          · {family.archived ? "Đã lưu trữ" : "Đang hoạt động"}
        </p>
        <p>
          Version testcase là nội dung nghiệp vụ; version automation là mã chạy;
          release R là bộ revision đã chốt. Chúng không tự cập nhật theo nhau.
        </p>
        <div
          className="decision-actions"
          role="group"
          aria-label="Nội dung revision"
        >
          {[
            ["edit", "Nội dung & duyệt"],
            ["history", "Lịch sử & so sánh"],
            ["runs", "Run & automation"],
          ].map(([key, label]) => (
            <button
              className="button secondary"
              aria-pressed={tab === key}
              key={key}
              onClick={() => setTab(key)}
            >
              {label}
            </button>
          ))}
        </div>
        <Link
          prefetch={false}
          href={`/documents/${item.document_set_id}?step=test-cases`}
        >
          Quay lại danh sách testcase
        </Link>{" "}
        ·{" "}
        <Link href={`/documents/${item.document_set_id}?step=use-export`}>
          Chốt release / xuất bộ
        </Link>
      </section>
      {error ? (
        <p role="alert" className="form-error">
          {error}{" "}
          <Link href={`/documents/${item.document_set_id}?step=requirements`}>
            Xem requirement nguồn
          </Link>
        </p>
      ) : null}
      {message ? (
        <p role="status" className="notice">
          {message}
        </p>
      ) : null}
      {tab === "edit" ? (
        <section className="panel panel-body">
          <h2>Nội dung revision</h2>
          {item.needs_source_review ? (
            <p className="notice">
              Nguồn đã thay đổi; cần đối chiếu requirement trước khi chốt bộ
              mới.
            </p>
          ) : null}
          {base.test_case.id !== family.head_revision_id ? (
            <p className="notice">
              Đang xem bản lịch sử. Mở latest để sửa hoặc phục hồi bản này thành
              nháp mới.
            </p>
          ) : null}
          <fieldset disabled={pending || !canEdit || family.archived}>
            <legend>Form bản nháp mới</legend>
            <div className="connect-grid">
              {editFields.map((key) => (
                <label key={key}>
                  <span>{fieldNames[key]}</span>
                  {key === "risk" || key === "test_type" ? (
                    <select
                      value={draft[key]}
                      onChange={(e) =>
                        setDraft({ ...draft, [key]: e.target.value })
                      }
                    >
                      {(key === "risk"
                        ? ["LOW", "MEDIUM", "HIGH"]
                        : [
                            "HAPPY",
                            "NEGATIVE",
                            "BOUNDARY",
                            "PERMISSION",
                            "STATE",
                            "INTEGRATION",
                            "REGRESSION",
                            "NFR",
                          ]
                      ).map((value) => (
                        <option key={value}>{value}</option>
                      ))}
                    </select>
                  ) : (
                    <textarea
                      aria-label={fieldNames[key]}
                      maxLength={16000}
                      value={draft[key]}
                      onChange={(e) =>
                        setDraft({ ...draft, [key]: e.target.value })
                      }
                    />
                  )}
                </label>
              ))}
            </div>
            <h3>Các bước có thứ tự</h3>
            <ol>
              {draft.steps.map((step, index) => (
                <li key={index}>
                  <label>
                    Thao tác bước {index + 1}
                    <textarea
                      aria-label={`Thao tác bước ${index + 1}`}
                      maxLength={16000}
                      value={step.action}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          steps: draft.steps.map((v, i) =>
                            i === index ? { ...v, action: e.target.value } : v,
                          ),
                        })
                      }
                    />
                  </label>
                  <label>
                    Expected bước {index + 1}
                    <textarea
                      aria-label={`Expected bước ${index + 1}`}
                      maxLength={16000}
                      value={step.expected_result}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          steps: draft.steps.map((v, i) =>
                            i === index
                              ? { ...v, expected_result: e.target.value }
                              : v,
                          ),
                        })
                      }
                    />
                  </label>
                  <button
                    type="button"
                    disabled={!index}
                    onClick={() => {
                      const steps = [...draft.steps];
                      [steps[index - 1], steps[index]] = [
                        steps[index],
                        steps[index - 1],
                      ];
                      setDraft({ ...draft, steps });
                    }}
                  >
                    Đưa bước {index + 1} lên
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      setDraft({
                        ...draft,
                        steps: draft.steps.filter((_, i) => i !== index),
                      })
                    }
                  >
                    Xóa bước {index + 1}
                  </button>
                </li>
              ))}
            </ol>
            <button
              type="button"
              disabled={draft.steps.length >= 200}
              onClick={() =>
                setDraft({
                  ...draft,
                  steps: [...draft.steps, { action: "", expected_result: "" }],
                })
              }
            >
              Thêm bước
            </button>
          </fieldset>
          <label className="revision-reason">
            Lý do / ghi chú
            <textarea
              aria-label="Lý do / ghi chú"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              maxLength={4000}
            />
          </label>
          <p>
            Requirement và citations được giữ từ bản nền; đổi expected phải có
            nguồn nghiệp vụ đã duyệt. Lưu nháp không tự duyệt, chạy test hay
            chuyển PASS.
          </p>
          {conflict ? (
            <section role="region" aria-label="Xung đột bản đang sửa">
              <TestcaseDiff
                familyID={family.id}
                from={base.test_case.id}
                to={conflict.test_case.id}
              />
              <h3>
                Head đã đổi sang v{conflict.test_case.version_number}; nội dung
                bạn nhập vẫn được giữ
              </h3>
              <DiffRows
                changes={[...editFields, "steps" as const]
                  .filter(
                    (key) =>
                      JSON.stringify(draft[key]) !==
                      JSON.stringify(draftOf(conflict)[key]),
                  )
                  .map((key) => ({
                    field: key,
                    before: draftOf(conflict)[key],
                    after: draft[key],
                  }))}
              />
              <button
                className="button secondary"
                onClick={() => {
                  setBase(conflict);
                  setConflict(undefined);
                  setError("");
                  command.current = undefined;
                }}
              >
                Đã đối chiếu, dùng head mới làm nền và giữ form
              </button>
            </section>
          ) : null}
          <div className="decision-actions">
            <button
              className="button"
              disabled={
                pending ||
                !canEdit ||
                family.archived ||
                !dirty ||
                Boolean(conflict) ||
                base.test_case.id !== family.head_revision_id
              }
              onClick={() => void mutate("save")}
            >
              Lưu bản nháp mới
            </button>
            <label>
              Tên người duyệt
              <input
                value={reviewer}
                onChange={(e) => setReviewer(e.target.value)}
                maxLength={160}
              />
            </label>
            <button
              className="button"
              disabled={pending || !canReview || dirty || family.archived}
              onClick={() => void mutate("APPROVED")}
            >
              Duyệt phiên bản
            </button>
            <button
              className="button secondary"
              disabled={pending || !canReview || dirty || family.archived}
              onClick={() => void mutate("REJECTED")}
            >
              Từ chối
            </button>
            <button
              className="button secondary"
              disabled={pending || !canReview || dirty}
              onClick={() => void exportRevision()}
            >
              Xuất đúng testcase v{item.version_number}
            </button>
          </div>
          <details>
            <summary>Lịch sử duyệt revision này</summary>
            {detail.reviews.map((review) => (
              <p key={review.id}>
                {review.reviewer_name} · {review.decision} · {review.comment}
              </p>
            ))}
          </details>
        </section>
      ) : null}
      {tab === "history" ? (
        <section className="panel panel-body">
          <h2>Timeline version</h2>
          <ol>
            {history.entries.map(({ revision: v, releases }) => (
              <li key={v.id}>
                <Link
                  prefetch={false}
                  href={`/documents/${v.document_set_id}/test-cases/${v.id}`}
                >
                  {v.test_case_key} v{v.version_number} · #{v.id}
                </Link>{" "}
                · <StatusBadge status={v.status} />
                <p>
                  {v.created_by} · {new Date(v.created_at).toLocaleString()} ·{" "}
                  {v.change_reason}
                </p>
                <p>
                  Source #{v.source_snapshot_id ?? "—"} · {v.generated_by} ·
                  model:{" "}
                  {String(
                    (v.provenance as Record<string, unknown>)?.model ??
                      "chưa ghi nhận",
                  )}{" "}
                  {v.restored_from_revision_id
                    ? `· phục hồi từ #${v.restored_from_revision_id}`
                    : ""}
                </p>
                <p>
                  Release refs (tối đa 50):{" "}
                  {releases.map((r) => `R${r.number}`).join(", ") ||
                    "chưa chốt"}
                </p>
                <button
                  onClick={() => {
                    setFrom(family.head_revision_id ?? item.id);
                    setTo(v.id);
                    setRestoreReady(false);
                  }}
                >
                  Đối chiếu / phục hồi v{v.version_number}
                </button>
              </li>
            ))}
          </ol>
          <div className="decision-actions">
            <button
              disabled={!historyBefore}
              onClick={() => setHistoryBefore(0)}
            >
              Mới nhất
            </button>
            <button
              disabled={!history.next_before}
              onClick={() => setHistoryBefore(history.next_before ?? 0)}
            >
              Version cũ hơn
            </button>
          </div>
          <div className="connect-grid">
            <label>
              Revision trước
              <input
                type="number"
                min={1}
                value={from}
                onChange={(e) => {
                  setFrom(Number(e.target.value));
                  setRestoreReady(false);
                }}
              />
            </label>
            <label>
              Revision sau / bản phục hồi
              <input
                type="number"
                min={1}
                value={to}
                onChange={(e) => {
                  setTo(Number(e.target.value));
                  setRestoreReady(false);
                }}
              />
            </label>
          </div>
          <TestcaseDiff
            familyID={family.id}
            from={from}
            to={to}
            onReady={diffReady}
          />
          <label className="revision-reason">
            Lý do phục hồi / lưu trữ
            <textarea
              aria-label="Lý do phục hồi / lưu trữ"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              maxLength={4000}
            />
          </label>
          <p>
            Phục hồi #{to} thành DRAFT mới trên head #{family.head_revision_id};
            release/run đang dùng không đổi. Không mang theo approval, PASS hoặc
            automation artifact.
          </p>
          <div className="decision-actions">
            <button
              className="button"
              disabled={
                pending ||
                !canEdit ||
                family.archived ||
                !restoreReady ||
                from !== family.head_revision_id ||
                dirty
              }
              onClick={() => void mutate("restore")}
            >
              Phục hồi thành bản nháp mới
            </button>
            <button
              className="button secondary"
              disabled={pending || !canReview || dirty}
              onClick={() => {
                if (
                  window.confirm(
                    family.archived
                      ? "Mở lại identity?"
                      : "Lưu trữ identity? Lịch sử và release vẫn giữ nguyên.",
                  )
                )
                  void mutate("archive");
              }}
            >
              {family.archived ? "Mở lại identity" : "Lưu trữ identity"}
            </button>
          </div>
        </section>
      ) : null}
      {tab === "runs" ? (
        <section className="panel panel-body">
          <h2>Run của testcase v{item.version_number}</h2>
          <label>
            Phạm vi kết quả
            <select
              value={scope}
              onChange={(e) => {
                setScope(e.target.value);
                setRunBefore(0);
              }}
            >
              <option value="revision">Chỉ revision đang xem</option>
              <option value="family">
                Toàn identity (ghi rõ từng revision)
              </option>
            </select>
          </label>
          {runsLoading ? (
            <p role="status">Đang tải kết quả chạy…</p>
          ) : runsError ? (
            <p role="alert">{runsError}</p>
          ) : runs.length ? (
            runs.map((run) => (
              <article key={run.id}>
                <h3>
                  Run #{run.run_id} · testcase v{run.version} ·{" "}
                  <StatusBadge status={run.status} />
                </h3>
                <p>
                  Artifact:{" "}
                  {run.artifact_id ? `#${run.artifact_id}` : "không có"}
                </p>
                <p>Expected: {run.expected}</p>
                <p>Actual: {run.actual || "Chưa có kết quả"}</p>
              </article>
            ))
          ) : (
            <p>
              Chưa chạy trong phạm vi này. Expected giống nhau không có nghĩa
              revision mới đã PASS.
            </p>
          )}
          <button disabled={!runBefore} onClick={() => setRunBefore(0)}>
            Run mới nhất
          </button>
          <button
            disabled={!nextRun}
            onClick={() => setRunBefore(nextRun ?? 0)}
          >
            Run cũ hơn
          </button>
          <h3>Automation chỉ của revision #{item.id}</h3>
          <AutomationHistory
            history={automation}
            expectedHash={item.expected_result_hash}
            canReview={canReview}
          />
        </section>
      ) : null}
    </div>
  );
}
