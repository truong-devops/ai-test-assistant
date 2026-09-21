import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { RequirementInventory } from "@/components/requirement-inventory";
import { RequirementExtractionAction } from "@/components/requirement-extraction-action";
import { DocumentWorkflowNav } from "@/components/document-workflow-nav";
import { ApiError, getDocuments, getDocumentSet, getDocumentWorkflow, getOpenQuestions, getRequirement, getRequirementConflicts, getRequirements } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function RequirementsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let set;
  try {
    set = await getDocumentSet(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const [requirements, conflicts, questions, documents, workflow] = await Promise.all([
    getRequirements(id), getRequirementConflicts(id), getOpenQuestions(id), getDocuments(id),
    getDocumentWorkflow(id),
  ]);
  const extraction = workflow.active_jobs.find((job) => job.operation === "EXTRACT_REQUIREMENTS") ??
    workflow.recent_jobs.find((job) => job.operation === "EXTRACT_REQUIREMENTS");
  const conflictDetails = await Promise.all(conflicts.map(async (conflict) => ({ conflict,
    left: await getRequirement(conflict.left_requirement_id), right: await getRequirement(conflict.right_requirement_id),
  })));
  const approved = requirements.filter((item) => item.status === "APPROVED").length;
  const drafts = requirements.filter((item) => item.status === "DRAFT").length;
  return (
    <AppShell active="documents">
      <DocumentWorkflowNav setId={set.id} workflow={workflow} active="requirements" />
      <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><span>Requirements</span></div>
      <div className="page-heading"><div><p className="eyebrow">Phase 4 · Human-owned baseline</p><h1>Requirement inventory</h1><p className="page-description">Requirements are extracted from semantic units, remain draft by default, and carry citations back to immutable source blocks.</p></div><RequirementExtractionAction setId={id} initialJob={extraction} canExtract={workflow.capabilities.can_extract} canRetry={workflow.capabilities.can_retry_job} canCancel={workflow.capabilities.can_cancel_job} /></div>
      <div className="summary-grid"><article className="stat-card accent"><p>Total inventory</p><strong>{requirements.length}</strong><small>Stored requirements</small></article><article className="stat-card"><p>Approved baseline</p><strong>{approved}</strong><small>Eligible for test generation</small></article><article className="stat-card"><p>Draft</p><strong>{drafts}</strong><small>Awaiting PO/BA review</small></article><article className={conflicts.length + questions.length ? "stat-card warning" : "stat-card"}><p>Needs clarification</p><strong>{conflicts.length + questions.length}</strong><small>{conflicts.length} conflicts · {questions.length} TBD</small></article></div>
      {requirements.length ? <RequirementInventory setId={set.id} requirements={requirements} documents={documents} /> : <EmptyState title="No requirement inventory" message="Build the semantic index first, then extract requirements. No source is approved automatically." />}
      {conflictDetails.length ? <section className="document-preview-section"><div className="section-heading"><div><h2>Source conflicts</h2><p>Conflicting statements remain separate until a reviewer resolves them.</p></div></div><div className="conflict-list">{conflictDetails.map(({ conflict, left, right }) => <article className="panel" key={conflict.id}><div className="panel-header"><div><h3>Conflict #{conflict.id}</h3><p>{conflict.reason}</p></div></div><div className="conflict-grid"><div><strong>{left.requirement.requirement_key}</strong><p>{left.requirement.statement}</p>{left.evidence.map((item) => <code key={item.id}>{item.document_name} v{item.version_number} · {item.source_locator}</code>)}</div><div><strong>{right.requirement.requirement_key}</strong><p>{right.requirement.statement}</p>{right.evidence.map((item) => <code key={item.id}>{item.document_name} v{item.version_number} · {item.source_locator}</code>)}</div></div></article>)}</div></section> : null}
      {questions.length ? <section className="document-preview-section panel"><div className="panel-header"><div><h2>Open questions for PO/BA</h2><p>TBD details are excluded from the approved coverage denominator.</p></div></div><div className="question-list">{questions.map((item) => <article key={item.id}><strong>{item.owner_role || "PO/BA"}</strong><p>{item.question}</p></article>)}</div></section> : null}
    </AppShell>
  );
}
