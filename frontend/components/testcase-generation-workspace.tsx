"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type {
  AIBudgetStatus,
  DocumentWorkflow,
  DocumentWorkflowJob,
  GenerationProposal,
  ProposalComparison,
  ProposalPage,
  Requirement,
  TestCaseRevisionContent,
} from "@/lib/types";
import { WorkflowOperationAction } from "@/components/workflow-operation-action";
import { DiffRows, DiffValue } from "@/components/testcase-diff";

const labels: Record<GenerationProposal["classification"], string> = {
  NEW_CASE: "Case mới",
  NEW_REVISION: "Revision mới",
  UNCHANGED: "Không đổi",
  RETIRE_CANDIDATE: "Đề xuất lưu trữ",
  AMBIGUOUS_MATCH: "Cần xác nhận identity",
};
const decisions: Record<GenerationProposal["classification"], string[]> = {
  NEW_CASE: ["CREATE_NEW"],
  NEW_REVISION: ["REVISE"],
  UNCHANGED: ["KEEP"],
  RETIRE_CANDIDATE: ["ARCHIVE"],
  AMBIGUOUS_MATCH: ["REVISE", "CREATE_NEW"],
};
const decisionLabels: Record<string, string> = {
  CREATE_NEW: "Tạo identity mới · bản nháp",
  REVISE: "Tạo revision nháp cho identity đã chọn",
  KEEP: "Giữ revision đã pin · không tạo bản mới",
  ARCHIVE: "Lưu trữ identity · giữ lịch sử",
  DISMISS: "Bỏ qua đề xuất này",
};
async function read<T>(route: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api/backend/api/${route}`, {
    cache: "no-store",
    signal,
  });
  if (!response.ok)
    throw new Error(
      `Không tải được dữ liệu (${response.status}). Kiểm tra kết nối và migration 30.`,
    );
  return response.json();
}

export function TestcaseGenerationWorkspace({
  setId,
  requirements,
  workflow,
  budget,
  affectedRevisionCount,
}: {
  setId: number;
  requirements: Requirement[];
  workflow: DocumentWorkflow;
  budget?: AIBudgetStatus;
  affectedRevisionCount: number;
}) {
  const eligible = requirements.filter(
    (r) =>
      r.status === "APPROVED" &&
      r.source_state === "CURRENT" &&
      !r.review_blockers?.length,
  );
  const scopeSummary = workflow.generation_scope;
  const [scope, setScope] = useState(
    scopeSummary?.default_scope.toLowerCase() ?? "",
  );
  useEffect(() => {
    setScope(workflow.generation_scope?.default_scope.toLowerCase() ?? "");
  }, [workflow.source_revision, setId]);
  const affectedIDs = scopeSummary?.affected_requirement_ids ?? [];
  const retireCount =
    scope === "selected" ? 0 : (scopeSummary?.removed_family_ids.length ?? 0);
  const [selected, setSelected] = useState<number[]>([]);
  const scopeCount =
    scope === "affected"
      ? affectedIDs.length
      : scope === "all"
        ? eligible.length
        : selected.length;
  const [query, setQuery] = useState("");
  const [requirementPage, setRequirementPage] = useState(0);
  const [confirmed, setConfirmed] = useState(false);
  const [jobFilter, setJobFilter] = useState("");
  const [cursor, setCursor] = useState<number[]>([0]);
  const [epoch, setEpoch] = useState(0);
  const [page, setPage] = useState<ProposalPage>();
  const [error, setError] = useState("");
  const [opened, setOpened] = useState<GenerationProposal>();
  const generation =
    workflow.active_jobs.find((j) => j.operation === "GENERATE_TESTCASES") ??
    workflow.recent_jobs.find((j) => j.operation === "GENERATE_TESTCASES");
  const filtered = eligible.filter((r) =>
    `${r.requirement_key} ${r.title}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  const visible = filtered.slice(
    requirementPage * 20,
    (requirementPage + 1) * 20,
  );
  const selectionValid =
    selected.length > 0 &&
    selected.every((id) => eligible.some((r) => r.id === id));
  const payload =
    scope === "selected"
      ? {
          review_proposals: true,
          generation_scope: "SELECTED",
          requirement_ids: [...selected].sort((a, b) => a - b),
        }
      : { review_proposals: true, generation_scope: scope.toUpperCase() };
  const signature = JSON.stringify(payload);
  const eligibilitySignature = JSON.stringify([
    eligible.map((r) => [r.id, r.review_hash]),
    scopeSummary?.affected_requirement_ids,
    scopeSummary?.removed_family_ids,
  ]);
  useEffect(() => {
    setConfirmed(false);
  }, [scope, signature, workflow.source_revision, eligibilitySignature]);
  useEffect(() => {
    const abort = new AbortController();
    setPage(undefined);
    setError("");
    if (
      jobFilter &&
      (!/^[1-9]\d*$/.test(jobFilter) ||
        !Number.isSafeInteger(Number(jobFilter)))
    ) {
      setError("Job ID phải là số nguyên dương.");
      return;
    }
    read<ProposalPage>(
      `document-sets/${setId}/test-case-proposals?limit=20&before=${cursor[cursor.length - 1]}&job_id=${jobFilter || 0}`,
      abort.signal,
    )
      .then((value) => {
        if (!abort.signal.aborted) setPage(value);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(e.message);
      });
    return () => abort.abort();
  }, [setId, jobFilter, cursor, epoch]);
  const jobChanged = (job: DocumentWorkflowJob) => {
    setJobFilter(String(job.id));
    setCursor([0]);
    setEpoch((v) => v + 1);
    setConfirmed(false);
  };
  return (
    <section
      className="panel revision-workspace generation-workspace"
      aria-label="Sinh và đối chiếu đề xuất"
    >
      <div className="panel-header">
        <div>
          <h2>Sinh và đối chiếu đề xuất</h2>
          <p>
            AI chỉ tạo đề xuất. Reviewer áp dụng thành bản nháp; duyệt testcase
            và chốt release là các bước riêng.
          </p>
        </div>
      </div>
      <div className="panel-body">
        <p>
          Nguồn revision #{workflow.source_revision} · {eligible.length} yêu cầu
          đủ điều kiện · {affectedRevisionCount} revision hiện được đánh dấu cần
          đối chiếu nguồn.
        </p>
        <p>
          <Link href={`/documents/${setId}/requirements`}>
            Xem thay đổi nguồn và quan hệ testcase
          </Link>
          . Mặc định sau khi đã có testcase: yêu cầu chưa có head liên kết hiện
          hành hoặc head có nguồn không còn hiện hành. Mapping mơ hồ vẫn cần
          reviewer xác nhận identity, không tự ghép testcase.
        </p>
        <p>
          Phạm vi bị ảnh hưởng: {affectedIDs.length} yêu cầu đủ điều kiện ·{" "}
          {scopeSummary?.affected_family_ids.length ?? 0} identities cần đối
          chiếu · {scopeSummary?.removed_family_ids.length ?? 0} identities đã
          mất toàn bộ nguồn · {scopeSummary?.blocked_requirements ?? 0} yêu cầu
          còn cần duyệt/làm rõ.
        </p>
        {scope === "affected" ? (
          <p>
            {affectedIDs.map((id) => (
              <Link key={id} href={`/documents/${setId}/requirements/${id}`}>
                Requirement #{id}{" "}
              </Link>
            ))}
            {!scopeCount && !retireCount
              ? "Không có thay đổi đủ điều kiện để sinh; xem nguồn cần duyệt hoặc chọn lại phạm vi."
              : null}
          </p>
        ) : null}
        {scopeSummary?.changes.length ? (
          <details>
            <summary>
              Thay đổi nguồn hiện hành · {scopeSummary.changes.length} mục
            </summary>
            {scopeSummary.changes.map((change, index) => (
              <article key={index}>
                <strong>{change.classification}</strong>
                <p>{change.reason}</p>
                <p>
                  Nguồn cũ:{" "}
                  {change.before_ids.map((id) => (
                    <Link
                      key={id}
                      href={`/documents/${setId}/requirements/${id}`}
                    >
                      #{id}{" "}
                    </Link>
                  ))}{" "}
                  → mới:{" "}
                  {change.after_ids.map((id) => (
                    <Link
                      key={id}
                      href={`/documents/${setId}/requirements/${id}`}
                    >
                      #{id}{" "}
                    </Link>
                  ))}
                </p>
              </article>
            ))}
          </details>
        ) : null}
        {budget ? (
          <p>
            Ngân sách hiện có: đã dùng {budget.used_tokens.toLocaleString()}{" "}
            tokens, đang giữ {budget.reserved_tokens.toLocaleString()}, còn{" "}
            {budget.token_budget
              ? budget.remaining_tokens.toLocaleString()
              : "không giới hạn"}
            . Chi phí đã ghi nhận $
            {(budget.used_cost_microusd / 1e6).toFixed(4)}; còn{" "}
            {budget.cost_budget_microusd
              ? `$${(budget.remaining_cost_microusd / 1e6).toFixed(4)}`
              : "không giới hạn"}
            . Đây không phải ước lượng cho lần chạy mới.
          </p>
        ) : (
          <p>
            Ngân sách được server kiểm tra khi gửi; vai trò hiện tại không có
            quyền xem chi tiết ngân sách.
          </p>
        )}
        <label>
          Phạm vi sinh đề xuất
          <select
            aria-label="Phạm vi sinh đề xuất"
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            disabled={!workflow.capabilities.can_generate}
          >
            <option value="">Chọn phạm vi rõ ràng…</option>
            <option value="affected">
              Case bị ảnh hưởng / yêu cầu mới (mặc định cập nhật)
            </option>
            <option value="selected">Yêu cầu đã chọn (tối đa 100)</option>
            <option value="all">Toàn bộ yêu cầu hiện hành đã duyệt</option>
          </select>
        </label>
        {scope === "selected" ? (
          <fieldset>
            <legend>Chọn yêu cầu cho lần chạy này</legend>
            <input
              aria-label="Tìm yêu cầu để sinh đề xuất"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setRequirementPage(0);
              }}
              placeholder="Mã hoặc tiêu đề"
            />
            <p>
              Đã chọn {selected.length}/100; giữ lựa chọn ngoài trang, không tự
              chọn tất cả.
            </p>
            {visible.map((r) => (
              <label className="proposal-requirement" key={r.id}>
                <input
                  type="checkbox"
                  checked={selected.includes(r.id)}
                  disabled={
                    !workflow.capabilities.can_generate ||
                    (!selected.includes(r.id) && selected.length >= 100)
                  }
                  onChange={() =>
                    setSelected((old) =>
                      old.includes(r.id)
                        ? old.filter((id) => id !== r.id)
                        : [...old, r.id],
                    )
                  }
                />
                {r.requirement_key} v{r.version_number} · {r.title}
              </label>
            ))}
            {!visible.length ? (
              <p>Không có yêu cầu đủ điều kiện khớp bộ lọc.</p>
            ) : null}
            <div className="decision-actions">
              <button
                disabled={!requirementPage}
                onClick={() => setRequirementPage((v) => v - 1)}
              >
                Yêu cầu trước
              </button>
              <button
                disabled={(requirementPage + 1) * 20 >= filtered.length}
                onClick={() => setRequirementPage((v) => v + 1)}
              >
                Yêu cầu tiếp
              </button>
              <button onClick={() => setSelected([])}>Bỏ chọn yêu cầu</button>
            </div>
            {selected.length > 0 && !selectionValid ? (
              <p role="alert">
                Có yêu cầu đã hết điều kiện; bỏ chọn và chọn lại trước khi gửi.
              </p>
            ) : null}
          </fieldset>
        ) : null}
        {scope ? (
          <label className="proposal-requirement">
            <input
              type="checkbox"
              checked={confirmed}
              onChange={(e) => setConfirmed(e.target.checked)}
            />
            Tôi xác nhận sinh đề xuất cho {scopeCount} yêu cầu và {retireCount}{" "}
            identity có thể lưu trữ; chưa thay testcase, release hay chạy
            sandbox.
          </label>
        ) : null}
        <WorkflowOperationAction
          setId={setId}
          operation="GENERATE_TESTCASES"
          label="Sinh đề xuất để review"
          activeLabel="Đang sinh đề xuất…"
          payload={payload}
          initialJob={generation}
          disabled={
            !workflow.capabilities.can_generate ||
            !scope ||
            !confirmed ||
            (!scopeCount && !retireCount) ||
            (scope === "selected" && !selectionValid)
          }
          canRetry={workflow.capabilities.can_retry_job}
          canCancel={workflow.capabilities.can_cancel_job}
          onJob={jobChanged}
        />
        <p>
          Retry chỉ chạy requirement/retire unit chưa hoàn tất; proposal và
          quyết định đã lưu được giữ nguyên. Unit đã gọi AI nhưng chưa commit
          checkpoint có thể phát sinh phí lại. Job legacy cũ vẫn retry cả
          operation.
        </p>
      </div>
      <div className="panel-body">
        <h3>Đề xuất đã lưu</h3>
        <div className="filter-strip">
          <label>
            Lọc theo job ID
            <input
              inputMode="numeric"
              value={jobFilter}
              placeholder="Tất cả các job"
              onChange={(e) => {
                setJobFilter(e.target.value);
                setCursor([0]);
              }}
            />
          </label>
          <button onClick={() => setEpoch((v) => v + 1)}>
            Tải lại đề xuất
          </button>
        </div>
        {error ? (
          <p role="alert">{error}</p>
        ) : !page ? (
          <p role="status">Đang tải đề xuất…</p>
        ) : (
          <>
            <div
              className="table-wrap"
              tabIndex={0}
              aria-label="Danh sách đề xuất"
            >
              <table className="data-table">
                <caption>
                  Trang {cursor.length} · tối đa 20 đề xuất; không tự áp dụng
                  nhóm
                </caption>
                <thead>
                  <tr>
                    <th scope="col">Đề xuất</th>
                    <th scope="col">Phân loại</th>
                    <th scope="col">Trạng thái</th>
                    <th scope="col">Xem</th>
                  </tr>
                </thead>
                <tbody>
                  {page.proposals.map((p) => (
                    <tr key={p.id}>
                      <td>
                        #{p.id} · {p.content.title}
                        <span className="table-subtitle">
                          Job #{p.workflow_job_id} · nguồn #{p.source_revision}
                        </span>
                      </td>
                      <td>
                        {labels[p.classification]}
                        <span className="table-subtitle">{p.reason}</span>
                      </td>
                      <td>
                        {p.status === "PENDING"
                          ? "Chờ xử lý"
                          : p.status === "APPLIED"
                            ? "Đã áp dụng"
                            : "Đã bỏ qua"}
                      </td>
                      <td>
                        <button onClick={() => setOpened(p)}>
                          Xem đề xuất #{p.id}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!page.proposals.length ? (
              <p>
                Chưa có đề xuất trong phạm vi này. Job legacy tạo draft trực
                tiếp không có proposal.
              </p>
            ) : null}
            <div className="decision-actions">
              <button
                disabled={cursor.length === 1}
                onClick={() => setCursor((old) => old.slice(0, -1))}
              >
                Đề xuất trước
              </button>
              <button
                disabled={!page.next_before}
                onClick={() => setCursor((old) => [...old, page.next_before!])}
              >
                Đề xuất tiếp
              </button>
            </div>
          </>
        )}
      </div>
      {opened ? (
        <ProposalReview
          key={opened.id}
          proposal={opened}
          canReview={workflow.capabilities.can_review}
          onChanged={(p) => {
            setOpened(p);
            setEpoch((v) => v + 1);
          }}
        />
      ) : null}
    </section>
  );
}

function ProposalReview({
  proposal: p,
  canReview,
  onChanged,
}: {
  proposal: GenerationProposal;
  canReview: boolean;
  onChanged: (p: GenerationProposal) => void;
}) {
  const router = useRouter();
  const [family, setFamily] = useState(
    p.candidates.length === 1 ? String(p.candidates[0].family_id) : "",
  );
  const target = p.candidates.find((c) => String(c.family_id) === family);
  const [comparison, setComparison] = useState<ProposalComparison>();
  const [readError, setReadError] = useState("");
  const [job, setJob] = useState<DocumentWorkflowJob>();
  const [epoch, setEpoch] = useState(0);
  const [decision, setDecision] = useState("");
  const [reason, setReason] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [currentHead, setCurrentHead] = useState<number>();
  const busy = useRef(false);
  const command = useRef<{ body: string; key: string } | undefined>(undefined);
  useEffect(() => {
    const abort = new AbortController();
    setComparison(undefined);
    setReadError("");
    setConfirmed(false);
    setJob(undefined);
    Promise.all([
      read<{ job: DocumentWorkflowJob }>(
        `document-workflow-jobs/${p.workflow_job_id}`,
        abort.signal,
      ),
      target
        ? read<ProposalComparison>(
            `test-case-proposals/${p.id}/comparison?target_family_id=${target.family_id}`,
            abort.signal,
          )
        : Promise.resolve(undefined),
    ])
      .then(([status, diff]) => {
        if (!abort.signal.aborted) {
          setJob(status.job);
          setComparison(diff);
        }
      })
      .catch((e) => {
        if (!abort.signal.aborted) setReadError(e.message);
      });
    return () => abort.abort();
  }, [p.id, p.workflow_job_id, family, epoch]);
  const needsTarget = ["REVISE", "KEEP", "ARCHIVE"].includes(decision);
  const apply = async () => {
    if (busy.current || !confirmed || !reason.trim() || !decision) return;
    busy.current = true;
    setPending(true);
    setError("");
    const body = JSON.stringify({
      decision,
      reason: reason.trim(),
      ...(needsTarget && target
        ? {
            target_family_id: target.family_id,
            expected_head_revision_id: target.revision_id,
          }
        : {}),
    });
    if (command.current?.body !== body)
      command.current = { body, key: crypto.randomUUID() };
    try {
      const response = await fetch(
        `/api/backend/api/test-case-proposals/${p.id}/review`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Idempotency-Key": command.current.key,
          },
          body,
        },
      );
      const result = await response.json();
      if (!response.ok) {
        if (response.status === 409) {
          setConflict(true);
          setCurrentHead(result.current_revision_id);
        }
        throw new Error(
          response.status === 409
            ? "Đề xuất đã thay đổi trạng thái hoặc nguồn/head không còn như bản pin. Giữ nguyên lựa chọn; không tự rebase. Xem bản hiện tại hoặc bỏ qua và sinh đề xuất mới."
            : (result.message ??
              result.error ??
              `Không áp dụng được (${response.status}).`),
        );
      }
      onChanged(result);
      setConfirmed(false);
      window.dispatchEvent(new Event("document-workspace-updated"));
      router.refresh();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Mất kết nối. Thử lại cùng quyết định để dùng lại idempotency key.",
      );
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  const diffReady =
    comparison &&
    target &&
    comparison.target.revision_id === target.revision_id;
  const unitReady =
    job?.status === "SUCCEEDED" ||
    (job?.status === "PARTIAL_FAILED" &&
      job.units?.some(
        (unit) => unit.id === p.workflow_unit_id && unit.status === "SUCCEEDED",
      ));
  return (
    <section
      className="panel-body proposal-review"
      aria-label={`Review đề xuất #${p.id}`}
    >
      <h3>
        Đề xuất #{p.id} · {labels[p.classification]}
      </h3>
      <p>{p.reason}</p>
      <p>
        Job #{p.workflow_job_id}: {job?.status ?? "chưa tải"}. Nguồn đã pin #
        {p.source_revision}. Reviewer xác nhận identity, không suy đoán từ tiêu
        đề giống nhau.
      </p>
      <div className="decision-actions">
        {p.content.requirement_revision_ids.map((id) => (
          <Link
            key={id}
            href={`/documents/${p.document_set_id}/requirements/${id}`}
            target="_blank"
            rel="noreferrer"
          >
            Requirement #{id} (tab mới)
          </Link>
        ))}
      </div>
      <details open>
        <summary>Nội dung và citations của đề xuất</summary>
        <DiffValue value={p.content} />
      </details>
      <details>
        <summary>Generation provenance</summary>
        <DiffValue value={p.generation} />
      </details>
      {p.candidates.length ? (
        <>
          <label>
            Identity để đối chiếu
            <select
              aria-label="Identity để đối chiếu"
              value={family}
              disabled={pending}
              onChange={(e) => {
                setFamily(e.target.value);
                setConfirmed(false);
              }}
            >
              <option value="">Chọn identity, không tự ghép…</option>
              {p.candidates.map((c) => (
                <option key={c.family_id} value={c.family_id}>
                  Identity #{c.family_id} · revision #{c.revision_id} đã pin
                </option>
              ))}
            </select>
          </label>
          {target ? (
            <Link
              href={`/documents/${p.document_set_id}/test-cases/${target.revision_id}`}
              target="_blank"
              rel="noreferrer"
            >
              Mở revision #{target.revision_id} đã pin (tab mới)
            </Link>
          ) : null}
          {diffReady ? (
            <>
              <p>
                So với revision #{target.revision_id} đã pin, không phải head
                hiện tại. Apply sẽ kiểm tra head lại.
              </p>
              <DiffRows
                changes={(
                  Array.from(
                    new Set([
                      ...Object.keys(comparison.before),
                      ...Object.keys(comparison.after),
                    ]),
                  ) as (keyof TestCaseRevisionContent)[]
                )
                  .filter(
                    (key) =>
                      JSON.stringify(comparison.before[key] ?? null) !==
                      JSON.stringify(comparison.after[key] ?? null),
                  )
                  .map((key) => ({
                    field: key,
                    before: comparison.before[key],
                    after: comparison.after[key],
                  }))}
              />
            </>
          ) : target && !readError ? (
            <p role="status">Đang tải đối chiếu…</p>
          ) : null}
        </>
      ) : null}
      {readError ? (
        <p role="alert">
          {readError}{" "}
          <button onClick={() => setEpoch((v) => v + 1)}>
            Tải lại đối chiếu
          </button>
        </p>
      ) : (
        <button disabled={pending} onClick={() => setEpoch((v) => v + 1)}>
          Cập nhật trạng thái job / diff
        </button>
      )}
      {p.status !== "PENDING" ? (
        <p role="status">
          Đã ghi nhận: {decisionLabels[p.decision] ?? p.decision} ·{" "}
          {p.decided_by} · {p.decision_reason}.{" "}
          {p.result_revision_id ? (
            <Link
              href={`/documents/${p.document_set_id}/test-cases/${p.result_revision_id}`}
            >
              Mở revision kết quả #{p.result_revision_id} để xem / duyệt riêng
            </Link>
          ) : null}
        </p>
      ) : canReview ? (
        <fieldset disabled={pending}>
          <legend>Quyết định của reviewer</legend>
          <label>
            Cách xử lý đề xuất
            <select
              aria-label="Cách xử lý đề xuất"
              value={decision}
              onChange={(e) => {
                setDecision(e.target.value);
                setConfirmed(false);
              }}
            >
              <option value="">Chọn quyết định…</option>
              {[...decisions[p.classification], "DISMISS"].map((d) => (
                <option key={d} value={d}>
                  {decisionLabels[d]}
                </option>
              ))}
            </select>
          </label>
          <label>
            Lý do quyết định
            <textarea
              aria-label="Lý do quyết định"
              value={reason}
              maxLength={4000}
              onChange={(e) => {
                setReason(e.target.value);
                setConfirmed(false);
              }}
            />
          </label>
          <label className="proposal-requirement">
            <input
              type="checkbox"
              checked={confirmed}
              onChange={(e) => setConfirmed(e.target.checked)}
            />
            Tôi đã đọc nội dung/nguồn và xác nhận{" "}
            {decisionLabels[decision] ?? "quyết định trên"}
            {needsTarget && target
              ? ` tại identity #${target.family_id}, expected head #${target.revision_id}`
              : ""}
            . Không chuyển approval, PASS hoặc release binding.
          </label>
          <button
            className="button"
            disabled={
              !confirmed ||
              !reason.trim() ||
              !decision ||
              (decision !== "DISMISS" &&
                (conflict || !unitReady || (needsTarget && !diffReady)))
            }
            onClick={() => void apply()}
          >
            {pending ? "Đang lưu quyết định…" : "Xác nhận xử lý đề xuất"}
          </button>
          {job && !unitReady ? (
            <p>
              Unit/job chưa đủ điều kiện: chỉ có thể bỏ qua đề xuất; không apply
              vào bộ làm việc.
            </p>
          ) : null}
        </fieldset>
      ) : (
        <p>Chỉ reviewer/admin được áp dụng hoặc bỏ qua đề xuất.</p>
      )}
      {error ? (
        <p role="alert">
          {error}{" "}
          {currentHead ? (
            <Link
              href={`/documents/${p.document_set_id}/test-cases/${currentHead}`}
              target="_blank"
              rel="noreferrer"
            >
              Mở head hiện tại #{currentHead}
            </Link>
          ) : null}
        </p>
      ) : null}
    </section>
  );
}
