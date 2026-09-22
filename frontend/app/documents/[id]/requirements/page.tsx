import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell } from "@/components/shell";
import { RequirementReviewWorkspace } from "@/components/requirement-review-workspace";
import { DocumentWorkflowNav } from "@/components/document-workflow-nav";
import { ApiError, getDocuments, getDocumentSet, getDocumentWorkflow, getRequirements } from "@/lib/api";

export const dynamic = "force-dynamic";
export default async function RequirementsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let set;
  try { set = await getDocumentSet(id); } catch(error) { if(error instanceof ApiError && error.status===404)notFound(); throw error; }
  const [requirements, documents, workflow] = await Promise.all([getRequirements(id),getDocuments(id),getDocumentWorkflow(id)]);
  return <AppShell active="documents"><DocumentWorkflowNav key={set.id} setId={set.id} workflow={workflow} active="requirements" />
    <div className="breadcrumb"><Link href="/documents">Tài liệu</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><span>Yêu cầu</span></div>
    <RequirementReviewWorkspace key={set.id} setId={set.id} requirements={requirements} documents={documents} canReview={workflow.capabilities.can_review} />
  </AppShell>;
}
