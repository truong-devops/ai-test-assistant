import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { ReindexAction } from "@/components/index-action";
import { Stat } from "@/components/stat";
import { StatusBadge } from "@/components/status-badge";
import { ApiError, getAnalyses, getIndexStatus, getProject, optional } from "@/lib/api";
import { formatDate } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function ProjectDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let project;
  try {
    project = await getProject(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const [index, analyses] = await Promise.all([optional(() => getIndexStatus(project.id), null), optional(getAnalyses, [])]);
  const projectAnalyses = analyses.filter((analysis) => analysis.project_id === project.id);

  return (
    <AppShell active="projects">
      <div className="breadcrumb"><Link href="/projects">Projects</Link><span>/</span><span>{project.name}</span></div>
      <section className="project-hero">
        <div className="hero-identity"><span className="project-avatar" aria-hidden="true">{project.name.slice(0, 1).toUpperCase()}</span><div><p className="eyebrow">{project.provider === "github" ? "GitHub repository" : "GitLab project"} #{project.provider_project_id}</p><h1>{project.name}</h1><p>{project.repository_url}</p></div></div>
        <div className="hero-meta"><StatusBadge status={project.status} /></div>
      </section>
      <section className="panel">
        <div className="detail-grid">
          <div className="detail-cell"><span>Default branch</span><strong className="mono">{project.default_branch}</strong></div>
          <div className="detail-cell"><span>Language</span><strong>{project.language.toUpperCase()}</strong></div>
          <div className="detail-cell"><span>Connected</span><strong>{formatDate(project.created_at)}</strong></div>
        </div>
      </section>
      <div className="summary-grid" style={{ marginTop: 22 }}>
        <Stat label="Index state" value={<StatusBadge status={index?.status ?? "NOT_INDEXED"} />} detail={index ? `Generation ${index.generation} · ${index.embedding_model || "not indexed"}` : "No index requested"} />
        <Stat label="Indexed files" value={index?.file_count ?? 0} detail={`${index?.skipped_file_count ?? 0} skipped`} />
        <Stat label="Knowledge chunks" value={index?.chunk_count ?? 0} detail="Available to retrieval" />
        <Stat label="Analyses" value={projectAnalyses.length} detail="Merge-request jobs" />
      </div>
      <div className="content-grid">
        <section className="panel">
          <div className="panel-header"><div><h2>Analysis history</h2><p>Review-ready and in-flight merge-request jobs for this project.</p></div></div>
          {projectAnalyses.length ? <div className="table-wrap"><table className="data-table"><thead><tr><th>Merge / pull request</th><th>Status</th><th>Created</th></tr></thead><tbody>{projectAnalyses.map((analysis) => <tr key={analysis.id}><td><Link href={`/analyses/${analysis.id}`}><span className="table-title">{analysis.title || `Change request #${analysis.merge_request_iid}`}</span><span className="table-subtitle">Source {analysis.source_branch || "—"} → {analysis.target_branch || "—"}</span></Link></td><td><StatusBadge status={analysis.status} /></td><td>{formatDate(analysis.created_at)}</td></tr>)}</tbody></table></div> : <div className="panel-body"><EmptyState title="No analyses yet" message={`When ${project.provider === "github" ? "GitHub" : "GitLab"} delivers a change-request webhook, its trace will appear here.`} /></div>}
        </section>
        <aside className="stack"><section className="panel side-section"><p className="eyebrow">Knowledge index</p><h2>Refresh project evidence</h2><p className="page-description">A re-index creates a new generation safely. Existing review records remain intact.</p><div style={{ marginTop: 16 }}><ReindexAction projectId={project.id} /></div>{index?.error_message ? <p className="form-error">{index.error_message}</p> : null}</section></aside>
      </div>
    </AppShell>
  );
}
