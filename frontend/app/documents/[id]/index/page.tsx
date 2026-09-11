import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { WorkflowAction } from "@/components/workflow-action";
import { RetrievalDebug } from "@/components/index-workspace";
import { ApiError, getDocumentChunks, getDocumentIndex, getDocumentSet } from "@/lib/api";
import { humanize } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function DocumentIndexPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let set;
  try {
    set = await getDocumentSet(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const index = await getDocumentIndex(id);
  const chunks = index.status === "READY" ? await getDocumentChunks(id) : [];
  return (
    <AppShell active="documents">
      <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><span>Semantic index</span></div>
      <div className="page-heading"><div><p className="eyebrow">Phase 3 · Document RAG</p><h1>Semantic index</h1><p className="page-description">Chunks remain scoped to this document set and preserve immutable source locators.</p></div><WorkflowAction endpoint={`/api/document-sets/${id}/index`} label={index.status === "READY" ? "Re-index" : "Build index"} pendingLabel="Indexing…" /></div>
      <div className="summary-grid">
        <article className="stat-card accent"><p>Status</p><strong className="stat-badge"><StatusBadge status={index.status} /></strong><small>Generation {index.generation}</small></article>
        <article className="stat-card"><p>Versions</p><strong>{index.version_count}</strong><small>{index.skipped_version_count} skipped</small></article>
        <article className="stat-card"><p>Semantic chunks</p><strong>{index.chunk_count}</strong><small>{index.embedding_model || "Not embedded"}</small></article>
        <article className={index.warning_count ? "stat-card warning" : "stat-card"}><p>Warnings</p><strong>{index.warning_count}</strong><small>Draft sources or suspicious instructions</small></article>
      </div>
      {index.warning_count ? <p className="notice"><strong>Review required</strong>The index includes draft source versions or content resembling prompt injection. Retrieved text is always treated as untrusted evidence.</p> : null}
      {chunks.length ? <section className="panel">
        <div className="panel-header"><div><h2>Chunk inspector</h2><p>Grouped by document version, semantic type, and flow.</p></div><span className="section-counter">showing {chunks.length}</span></div>
        <div className="table-wrap"><table className="data-table"><thead><tr><th>Document / section</th><th>Identifier</th><th>Type</th><th>Flow</th><th>Source</th><th>Content</th></tr></thead><tbody>{chunks.map((chunk) => <tr key={chunk.id}><td>{String(chunk.metadata.document_name ?? "—")}<span className="table-subtitle">{chunk.title || "Root"}</span></td><td className="mono">{chunk.identifier || "—"}</td><td>{humanize(chunk.chunk_type)}</td><td><StatusBadge status={chunk.flow_type} /></td><td className="mono">v{chunk.document_version_id} · {chunk.source_locator}</td><td><span className="table-subtitle chunk-copy">{chunk.content}</span></td></tr>)}</tbody></table></div>
      </section> : <EmptyState title="No semantic chunks" message="Parse at least one source document, then build the document-set index." />}
      {index.status === "READY" ? <div className="document-preview-section"><RetrievalDebug setId={set.id} /></div> : null}
    </AppShell>
  );
}
