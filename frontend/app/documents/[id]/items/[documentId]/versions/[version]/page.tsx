import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { ApiError, getDocumentSet, getDocumentVersion } from "@/lib/api";
import { formatDate, humanize } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function DocumentVersionPage({ params }: {
  params: Promise<{ id: string; documentId: string; version: string }>;
}) {
  const { id, documentId, version: versionNumber } = await params;
  let detail;
  try {
    detail = await getDocumentVersion(documentId, versionNumber);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  if (detail.document.document_set_id !== Number(id)) notFound();
  const set = await getDocumentSet(id);
  const { document, version, blocks } = detail;
  return (
    <AppShell active="documents">
      <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><Link href={`/documents/${set.id}`}>{set.name}</Link><span>/</span><span>{document.name} v{version.version_number}</span></div>
      <section className="project-hero">
        <div className="hero-identity"><span className="project-avatar" aria-hidden="true">v{version.version_number}</span><div><p className="eyebrow">{humanize(document.document_type)}</p><h1>{document.name}</h1><p>{version.original_filename} · {version.size_bytes.toLocaleString("en-US")} bytes · SHA-256 <span className="mono">{version.sha256.slice(0, 12)}…</span></p></div></div>
        <div className="hero-meta"><StatusBadge status={version.parse_status} /><StatusBadge status={version.approval_status} /></div>
      </section>
      <section className="panel">
        <div className="detail-grid">
          <div className="detail-cell"><span>Version</span><strong>v{version.version_number}</strong></div>
          <div className="detail-cell"><span>Parsed blocks</span><strong>{version.block_count}</strong></div>
          <div className="detail-cell"><span>Uploaded</span><strong>{formatDate(version.uploaded_at)}</strong></div>
        </div>
      </section>
      {version.parse_error ? <p className="notice"><strong>Parser error</strong>{version.parse_error}</p> : null}
      <section className="document-preview-section">
        <div className="section-heading"><div><h2>Structured preview</h2><p>Every block keeps a stable locator back to the uploaded source.</p></div><span className="section-counter">{blocks.length} blocks</span></div>
        {blocks.length ? <div className="document-block-list">{blocks.map((block) => (
          <article className={`document-block block-${block.block_type.toLowerCase()}`} key={block.id}>
            <header><span>{block.ordinal}. {humanize(block.block_type)}</span><code>{block.source_locator}</code></header>
            {block.block_type === "HEADING" ? <h3>{block.content}</h3> : <pre>{block.content}</pre>}
          </article>
        ))}</div> : <EmptyState title="No parsed blocks yet" message={version.parse_status === "FAILED" ? "Parsing failed. Check the error above and upload a corrected new version." : "The worker has not completed parsing this version. Refresh after a moment."} />}
      </section>
    </AppShell>
  );
}
