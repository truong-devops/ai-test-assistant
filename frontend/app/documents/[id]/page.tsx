import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { DocumentWorkflowNav, type WorkspaceStep } from "@/components/document-workflow-nav";
import { DocumentSourceWorkspace } from "@/components/document-source-workspace";
import { DocumentLifecycle } from "@/components/document-lifecycle";
import { RequirementReviewWorkspace } from "@/components/requirement-review-workspace";
import { TestCaseWorkspace } from "@/components/test-case-workspace";
import { SuiteReleasePublisher } from "@/components/suite-release-publisher";
import { ExportControls } from "@/components/export-controls";
import { WorkflowOperationAction } from "@/components/workflow-operation-action";
import { TestcaseGenerationWorkspace } from "@/components/testcase-generation-workspace";
import { ApiError, documentMaxUploadBytes, getAIBudget, getDocumentSet, getDocuments,
  getDocumentWorkflow, getRequirements, getBusinessTestCases, getSuiteReleases, getTestExports } from "@/lib/api";
import { documentWorkflowCopy as copy } from "@/lib/document-workflow-copy";

export const dynamic = "force-dynamic";

export default async function DocumentSetPage({ params, searchParams }: {
  params: Promise<{ id: string }>; searchParams: Promise<{ step?: string }>;
}) {
  const { id } = await params;
  const query = await searchParams;
  const step: WorkspaceStep = ["requirements", "test-cases", "use-export"].includes(query.step ?? "") ? query.step as WorkspaceStep : "documents";
  let set;
  try { set = await getDocumentSet(id); } catch (error) { if (error instanceof ApiError && error.status === 404) notFound(); throw error; }
  const [documents, workflow] = await Promise.all([getDocuments(set.id), getDocumentWorkflow(set.id)]);
  const budget = workflow.capabilities.can_manage ? await getAIBudget(set.id) : undefined;
  const requirements = step === "requirements" || step === "test-cases" ? await getRequirements(set.id) : [];
  const cases = step === "test-cases" || step === "use-export" ? await getBusinessTestCases(set.id) : [];
  const [releases, exports] = step === "use-export" ? await Promise.all([getSuiteReleases(set.id), getTestExports(set.id)]) : [[], []];
  const blockerCopy: Record<string, string> = {
    NO_DOCUMENTS: copy.blockers.noDocuments, SOURCE_PARSE_PENDING: copy.blockers.parsing,
    INDEX_NOT_CURRENT: copy.blockers.indexStale, SOURCE_NOT_APPROVED: copy.blockers.sourceReviewRequired,
    NO_APPROVED_REQUIREMENTS: copy.blockers.noApprovedRequirements, AI_BUDGET_EXHAUSTED: copy.blockers.budgetExceeded,
    DOCUMENT_SET_INACTIVE: "Bộ tài liệu đã lưu trữ. Người quản lý cần khôi phục trước khi thực hiện thao tác mới.",
  };
  return <AppShell active="documents">
    <div className="breadcrumb"><Link href="/documents">Tài liệu</Link><span>/</span><span>{set.name}</span></div>
    <section className="project-hero"><div className="hero-identity"><span className="project-avatar" aria-hidden="true">D</span><div><p className="eyebrow">Bộ làm việc · {set.status === "ACTIVE" ? "Đang hoạt động" : "Đã lưu trữ"}</p><h1>{set.name}</h1><p>{[set.product_name, set.scope, set.description].filter(Boolean).join(" · ") || "Từ tài liệu nguồn đến bộ testcase được duyệt."}</p></div></div></section>
    <DocumentWorkflowNav key={set.id} setId={set.id} workflow={workflow} active={step} />
    <div id="workspace-content" className="workspace-content">
      {step === "documents" ? <DocumentSourceWorkspace key={set.id} setId={set.id} documents={documents} workflow={workflow} maxBytes={documentMaxUploadBytes} /> : null}
      {step !== "documents" && workflow.blocking_reasons.length ? <div className="notice"><strong>Điều kiện để tiếp tục</strong><ul>{workflow.blocking_reasons.map((reason) => <li key={reason.code}>{blockerCopy[reason.code] ?? reason.message}</li>)}</ul><Link href={`/documents/${set.id}?step=documents`}>Xem và xử lý nguồn</Link><p>Dữ liệu đã có vẫn xem được; các thao tác mới kiểm tra điều kiện tại server.</p></div> : null}
      {step === "requirements" ? <>
        <RequirementReviewWorkspace key={set.id} setId={set.id} requirements={requirements} documents={documents} canReview={workflow.capabilities.can_review} />
      </> : null}
      {step === "test-cases" ? <>
        <TestcaseGenerationWorkspace key={set.id} setId={set.id} requirements={requirements} workflow={workflow} budget={budget} affectedRevisionCount={cases.filter((c) => c.needs_source_review).length} />
        <TestCaseWorkspace setId={set.id} testCases={cases} canReview={workflow.capabilities.can_review} />
        <Link className="button secondary" href={`/documents/${id}/test-cases`}>Xem ma trận coverage và lịch sử kết quả</Link>
        <Link className="button" href={`/documents/${id}?step=use-export`}>Tiếp tục đến sử dụng / xuất</Link>
      </> : null}
      {step === "use-export" ? <>
        <p>Chốt bộ testcase để cố định các revision được sử dụng. Bộ đã chốt, bản nháp và kết quả chạy được xuất riêng.</p>
        {cases.length ? <>
          {workflow.capabilities.can_publish ? <SuiteReleasePublisher setId={set.id} suiteId={cases[0].test_suite_id} testCases={cases} releases={releases} /> : <p className="notice">Cần testcase đã duyệt và quyền reviewer để chốt bộ.</p>}
          {workflow.capabilities.can_review ? <ExportControls setId={set.id} suiteId={cases[0].test_suite_id} exports={exports} testCases={cases} releases={releases} /> : <p className="notice">Cần quyền reviewer để tạo bản xuất.</p>}
          <Link className="button secondary" href="/projects">Mở project để gắn bộ đã chốt</Link>
        </> : <EmptyState title="Chưa có testcase để sử dụng" message="Tạo và duyệt testcase ở bước 3. Testcase chưa chạy sẽ được ghi rõ là Chưa chạy." />}
      </> : null}
    </div>
    <details className="panel workflow-details"><summary>Chi tiết xử lý</summary><div className="panel-body"><Link href={`/documents/${id}/index`}>Mở trình kiểm tra dữ liệu tìm kiếm</Link>
      {workflow.recent_jobs.slice(0, 5).map((job) => <div key={job.id} className="workflow-job-detail"><p>Job #{job.id} · {job.operation} · {job.completed_units}/{job.total_units} · {job.status}</p>
        <WorkflowOperationAction key={`${job.id}:${job.status}:${job.revision}`} setId={id} operation={job.operation} label="Thực hiện qua bước nguồn / testcase" activeLabel="Đang xử lý…" initialJob={job} disabled canRetry={workflow.capabilities.can_retry_job} canCancel={workflow.capabilities.can_cancel_job} />
      </div>)}
      {workflow.source_intents.slice(0, 5).map((intent) => <p key={intent.id}>Yêu cầu #{intent.id} · {intent.command} · {intent.status}{intent.error_message ? ` · ${intent.error_message}` : ""}</p>)}
    </div></details>
    {budget && workflow.capabilities.can_manage ? <section aria-label="Quản lý bộ tài liệu"><h2>Quản lý bộ tài liệu</h2><DocumentLifecycle set={set} budget={budget} /></section> : null}
  </AppShell>;
}
