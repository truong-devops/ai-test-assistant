import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { UploadDocument } from "@/components/upload-document";
import { DocumentLifecycle } from "@/components/document-lifecycle";
import { ApiError, documentMaxUploadBytes, getAIBudget, getDocumentSet, getDocuments } from "@/lib/api";
import { formatDate, humanize } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function DocumentSetPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let set;
  try {
    set = await getDocumentSet(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const [documents, budget] = await Promise.all([getDocuments(set.id), getAIBudget(set.id)]);
  return (
    <AppShell active="documents">
      <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><span>{set.name}</span></div>
      <section className="project-hero">
        <div className="hero-identity"><span className="project-avatar" aria-hidden="true">D</span><div><p className="eyebrow">{set.product_name || `Document set #${set.id}`}</p><h1>{set.name}</h1><p>{[set.scope, set.description].filter(Boolean).join(" · ") || "No scope description has been added."}</p></div></div>
        <div className="hero-meta"><StatusBadge status={set.status} /></div>
      </section>
      <section className="workflow-links" aria-label="Document-driven workflow">
        <Link className="workflow-link" href={`/documents/${set.id}/index`}><strong>1. Semantic index</strong><span>Chunk inspector and retrieval diagnostics</span></Link>
        <Link className="workflow-link" href={`/documents/${set.id}/requirements`}><strong>2. Requirements</strong><span>Inventory, evidence, conflicts, and review</span></Link>
        <Link className="workflow-link" href={`/documents/${set.id}/test-cases`}><strong>3. Test cases</strong><span>Grounded cases and deterministic coverage</span></Link>
      </section>
      <DocumentLifecycle set={set} budget={budget} />
      {set.status === "ACTIVE" ? <UploadDocument setId={set.id} maxBytes={documentMaxUploadBytes} /> : <p className="notice">This document set is archived. Restore it before uploading another immutable version.</p>}
      {documents.length ? (
        <section className="panel">
          <div className="panel-header"><div><h2>Source documents</h2><p>The newest immutable version is shown for each logical document.</p></div><span className="section-counter">{documents.length} document{documents.length === 1 ? "" : "s"}</span></div>
          <div className="table-wrap"><table className="data-table">
            <thead><tr><th>Document</th><th>Type</th><th>Version</th><th>Parse</th><th>Approval</th><th>Uploaded</th></tr></thead>
            <tbody>{documents.map((item) => {
              const version = item.latest_version;
              const href = version ? `/documents/${set.id}/items/${item.id}/versions/${version.version_number}` : undefined;
              return <tr key={item.id}>
                <td>{href ? <Link href={href}><span className="table-title">{item.name}</span><span className="table-subtitle">{version?.original_filename}</span></Link> : <span className="table-title">{item.name}</span>}</td>
                <td>{humanize(item.document_type)}</td><td className="mono">{version ? `v${version.version_number}` : "—"}</td>
                <td><StatusBadge status={version?.parse_status ?? "NOT_PARSED"} /></td><td><StatusBadge status={version?.approval_status ?? "DRAFT"} /></td>
                <td>{formatDate(version?.uploaded_at)}</td>
              </tr>;
            })}</tbody>
          </table></div>
        </section>
      ) : <EmptyState title="No documents uploaded" message="Upload a DOCX or Markdown source above. The parser preserves headings, paragraphs, lists, tables, code blocks, and source locators." />}
    </AppShell>
  );
}
