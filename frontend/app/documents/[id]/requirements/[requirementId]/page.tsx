import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell } from "@/components/shell";
import { RequirementReview } from "@/components/requirement-review";
import { StatusBadge } from "@/components/status-badge";
import { ApiError, getDocumentSet, getRequirement } from "@/lib/api";
import { humanize } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function RequirementDetailPage({ params }: { params: Promise<{ id: string; requirementId: string }> }) {
  const { id, requirementId } = await params;
  let detail;
  try {
    detail = await getRequirement(requirementId);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  if (detail.requirement.document_set_id !== Number(id)) notFound();
  const set = await getDocumentSet(id);
  const req = detail.requirement;
  return (
    <AppShell active="documents">
      <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><Link href={`/documents/${id}/requirements`}>Requirements</Link><span>/</span><span>{req.requirement_key}</span></div>
      <section className="project-hero"><div className="hero-identity"><span className="project-avatar">R</span><div><p className="eyebrow">{humanize(req.requirement_type)} · {humanize(req.flow_type)}</p><h1>{req.title}</h1><p>{req.statement}</p></div></div><div className="hero-meta"><StatusBadge status={req.risk} /><StatusBadge status={req.status} /></div></section>
      <section className="panel"><div className="detail-grid"><div className="detail-cell"><span>Identifier</span><strong className="mono">{req.requirement_key}</strong></div><div className="detail-cell"><span>Version</span><strong>v{req.version_number}</strong></div><div className="detail-cell"><span>Actor</span><strong>{req.actor || "—"}</strong></div></div></section>
      <section className="document-preview-section panel"><div className="panel-header"><div><h2>Evidence citations</h2><p>Approval requires at least one citation from an approved source version.</p></div><span className="section-counter">{detail.evidence.length}</span></div><div className="evidence-drawer">{detail.evidence.map((item) => <article id={`evidence-${item.id}`} key={item.id}><header><strong>{item.document_name} v{item.version_number}</strong><StatusBadge status={item.approval_status} /><code>{item.source_locator}</code></header><pre>{item.excerpt}</pre></article>)}</div></section>
      {detail.flow_steps.length ? <section className="document-preview-section panel"><div className="panel-header"><h2>Structured flow</h2></div><ol className="step-list">{detail.flow_steps.map((step) => <li key={step.id}><strong>{step.action}</strong>{step.expected_result ? <p>Expected: {step.expected_result}</p> : null}</li>)}</ol></section> : null}
      <div className="document-preview-section"><RequirementReview requirement={req} /></div>
    </AppShell>
  );
}
