import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell } from "@/components/shell";
import { TestcaseRevisionWorkspace } from "@/components/testcase-revision-workspace";
import {
  ApiError,
  getAutomationHistory,
  getBusinessTestCase,
  getDocumentSet,
  getDocumentWorkflow,
  getTestCaseFamily,
} from "@/lib/api";

export const dynamic = "force-dynamic";
export default async function TestCaseDetailPage({
  params,
}: {
  params: Promise<{ id: string; testCaseId: string }>;
}) {
  const { id, testCaseId } = await params;
  let detail;
  try {
    detail = await getBusinessTestCase(testCaseId);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  if (detail.test_case.document_set_id !== Number(id)) notFound();
  const [set, family, workflow, automation] = await Promise.all([
    getDocumentSet(id),
    getTestCaseFamily(detail.test_case.family_id),
    getDocumentWorkflow(id),
    getAutomationHistory(testCaseId),
  ]);
  return (
    <AppShell active="documents">
      <div className="breadcrumb">
        <Link href="/documents">Tài liệu</Link>
        <span>/</span>
        <Link href={`/documents/${id}?step=test-cases`}>{set.name}</Link>
        <span>/</span>
        <span>
          {detail.test_case.test_case_key} v{detail.test_case.version_number}
        </span>
      </div>
      <h1>{detail.test_case.title}</h1>
      <TestcaseRevisionWorkspace
        key={detail.test_case.id}
        detail={detail}
        family={family}
        automation={automation}
        canEdit={workflow.capabilities.can_upload}
        canReview={workflow.capabilities.can_review}
      />
      <section className="panel panel-body">
        <h2>Requirement & evidence của revision này</h2>
        {detail.requirements.map((req) => (
          <p key={req.requirement_id}>
            <Link href={`/documents/${id}/requirements/${req.requirement_id}`}>
              {req.requirement_key}: {req.requirement_title}
            </Link>
          </p>
        ))}
        {detail.evidence.map((e) => (
          <article key={e.id}>
            <h3>
              {e.document_name} · nguồn v{e.version_number} ·{" "}
              {e.approval_status}
            </h3>
            <p>{e.source_locator}</p>
            <pre className="source-text">{e.excerpt}</pre>
          </article>
        ))}
      </section>
    </AppShell>
  );
}
