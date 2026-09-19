import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { DocumentIndexAction } from "@/components/document-index-action";
import { RetrievalDebug } from "@/components/index-workspace";
import { ApiError, getDocumentChunks, getDocumentIndex, getDocumentSet, getDocumentWorkflow } from "@/lib/api";
import { humanize } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function DocumentIndexPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ generation?: string }>;
}) {
  const { id } = await params;
  const query = await searchParams;
  let set;
  try {
    set = await getDocumentSet(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const [index, workflow] = await Promise.all([getDocumentIndex(id), getDocumentWorkflow(id)]);
  const indexJob = workflow.active_jobs.find((job) => job.operation === "INDEX_DOCUMENTS") ??
    workflow.recent_jobs.find((job) => job.operation === "INDEX_DOCUMENTS");
  const requestedGeneration = Number.parseInt(query.generation ?? "", 10);
  const selectedHistory = Number.isSafeInteger(requestedGeneration)
    ? index.generations.find((item) => item.generation === requestedGeneration)
    : undefined;
  const currentUsable = index.status === "READY" && index.freshness === "CURRENT";
  const displayedGeneration = selectedHistory?.generation ?? (currentUsable ? index.generation : 0);
  const displayed = index.generations.find((item) => item.generation === displayedGeneration);
  const chunks = displayedGeneration > 0 && displayed?.status === "READY"
    ? await getDocumentChunks(id, displayedGeneration)
    : [];
  const historical = displayedGeneration > 0 &&
    (displayedGeneration !== index.generation || index.freshness !== "CURRENT");

  return (
    <AppShell active="documents">
      <div className="breadcrumb"><Link href="/documents">Tài liệu</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><span>Dữ liệu tìm kiếm</span></div>
      <div className="page-heading">
        <div><p className="eyebrow">UV-01 · Document RAG</p><h1>Dữ liệu tìm kiếm theo phiên bản</h1><p className="page-description">Mỗi generation chỉ chứa chunk thuộc đúng snapshot nguồn đã chốt.</p></div>
        <DocumentIndexAction setId={id} hasGeneration={index.generation > 0}
          initialJob={indexJob} canIndex={workflow.capabilities.can_index}
          canRetry={workflow.capabilities.can_retry_job}
          canCancel={workflow.capabilities.can_cancel_job} />
      </div>

      {index.freshness === "STALE" && index.source_revision > 0 ? <div className="notice" role="status">
        <strong>Dữ liệu tìm kiếm cần cập nhật.</strong>
        Nguồn hiện tại là revision {index.source_revision}, trong khi generation gần nhất dùng revision {index.indexed_source_revision}.
        {index.pending_versions.length ? <span> Phiên bản chờ cập nhật: {index.pending_versions.map((item) => `${item.document_name} v${item.version_number}`).join(", ")}.</span> : null}
      </div> : null}
      {historical ? <div className="notice"><strong>Đang xem generation lịch sử {displayedGeneration}.</strong> Nội dung này chỉ để chẩn đoán và không được coi là dữ liệu current.</div> : null}
      {index.error_message ? <p className="form-error" role="alert">Lần xử lý gần nhất thất bại: {index.error_message}</p> : null}

      <div className="summary-grid">
        <article className="stat-card accent"><p>Trạng thái</p><strong className="stat-badge"><StatusBadge status={index.status} /></strong><small>{index.freshness === "CURRENT" ? "Đúng phiên bản nguồn" : "Cần cập nhật"}</small></article>
        <article className="stat-card"><p>Generation</p><strong>{index.generation}</strong><small>source revision {index.indexed_source_revision || "—"}</small></article>
        <article className="stat-card"><p>Semantic chunks</p><strong>{index.chunk_count}</strong><small>{index.embedding_model || "Chưa embedding"}</small></article>
        <article className={index.warning_count ? "stat-card warning" : "stat-card"}><p>Cảnh báo</p><strong>{index.warning_count}</strong><small>{index.extraction_ready ? "Sẵn sàng trích xuất" : "Kiểm tra nguồn trước khi trích xuất"}</small></article>
      </div>

      {index.sources.length ? <section className="panel">
        <div className="panel-header"><div><h2>Snapshot nguồn của generation hiện tại</h2><p>Phiên bản nghiệp vụ hiển thị riêng; database ID chỉ nằm trong chi tiết kỹ thuật.</p></div><span className="section-counter">snapshot #{index.source_snapshot_id}</span></div>
        <div className="table-wrap"><table className="data-table"><thead><tr><th>Tài liệu</th><th>Phiên bản</th><th>Parse</th><th>Duyệt</th><th>Phạm vi</th></tr></thead><tbody>{index.sources.map((source) => <tr key={source.document_id}><td>{source.document_name}<span className="table-subtitle">technical version id #{source.document_version_id}</span></td><td>v{source.version_number}</td><td><StatusBadge status={source.parse_status} /></td><td><StatusBadge status={source.approval_status} /></td><td>{source.included ? "Đã bao gồm" : `Đã loại: ${source.exclusion_reason}`}</td></tr>)}</tbody></table></div>
      </section> : null}

      <section className="panel document-preview-section">
        <div className="panel-header"><div><h2>Lịch sử generation</h2><p>Chọn một generation để xem đúng membership lịch sử của nó.</p></div><span className="section-counter">{index.generations.length}</span></div>
        {index.generations.length ? <div className="table-wrap"><table className="data-table"><thead><tr><th>Generation</th><th>Nguồn</th><th>Trạng thái</th><th>Chunks</th><th>Lỗi</th></tr></thead><tbody>{index.generations.map((generation) => <tr key={generation.generation}><td><Link href={`/documents/${id}/index?generation=${generation.generation}`}>Generation {generation.generation}</Link>{generation.generation === index.generation && index.freshness === "CURRENT" ? <span className="table-subtitle">Current</span> : <span className="table-subtitle">Lịch sử</span>}</td><td>revision {generation.source_revision}</td><td><StatusBadge status={generation.status} /></td><td>{generation.chunk_count}</td><td>{generation.error_message || "—"}</td></tr>)}</tbody></table></div> : <p className="panel-body table-subtitle">Chưa có generation nào.</p>}
      </section>

      {chunks.length ? <section className="panel document-preview-section">
        <div className="panel-header"><div><h2>Chunk inspector · generation {displayedGeneration}</h2><p>Chunk được giới hạn bằng membership của generation đang xem.</p></div><span className="section-counter">hiển thị {chunks.length}</span></div>
        <div className="table-wrap"><table className="data-table"><thead><tr><th>Tài liệu / mục</th><th>Identifier</th><th>Loại</th><th>Luồng</th><th>Nguồn</th><th>Nội dung</th></tr></thead><tbody>{chunks.map((chunk) => <tr key={chunk.id}><td>{String(chunk.metadata.document_name ?? "—")}<span className="table-subtitle">{chunk.title || "Root"}</span></td><td className="mono">{chunk.identifier || "—"}</td><td>{humanize(chunk.chunk_type)}</td><td><StatusBadge status={chunk.flow_type} /></td><td className="mono">v{chunk.document_version_number} · {chunk.source_locator}<span className="table-subtitle">technical id #{chunk.document_version_id}</span></td><td><span className="table-subtitle chunk-copy">{chunk.content}</span></td></tr>)}</tbody></table></div>
      </section> : <EmptyState title={displayedGeneration ? "Generation không có chunk hoàn chỉnh" : "Chưa có dữ liệu tìm kiếm current"} message={index.freshness === "STALE" ? "Hãy cập nhật dữ liệu tìm kiếm hoặc chọn một generation lịch sử để chẩn đoán." : "Hãy parse tài liệu và tạo dữ liệu tìm kiếm."} />}
      {displayed?.status === "READY" ? <div className="document-preview-section"><RetrievalDebug setId={set.id} generation={displayedGeneration} /></div> : null}
    </AppShell>
  );
}
