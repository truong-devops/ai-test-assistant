import Link from "next/link";
import { AppShell, EmptyState, PageHeading } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { ConnectProject } from "@/components/connect-project";
import { getAnalyses, getIndexStatus, getProjects, optional } from "@/lib/api";
import { formatDate } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function ProjectsPage() {
  const [projects, analyses] = await Promise.all([optional(getProjects, []), optional(getAnalyses, [])]);
  const indexes = new Map(await Promise.all(projects.map(async (project) => [
    project.id,
    await optional(() => getIndexStatus(project.id), null),
  ] as const)));

  return (
    <AppShell active="projects">
      <PageHeading eyebrow="GitLab & GitHub sources" title="Projects" description="View repository configuration, knowledge-index state and merge/pull-request analysis history." />
      <ConnectProject />
      {projects.length ? (
        <section className="panel">
          <div className="panel-header"><div><h2>Source project registry</h2><p>Index state and most recent analysis are shown side by side.</p></div><span className="section-counter">{projects.length} project{projects.length === 1 ? "" : "s"}</span></div>
          <div className="table-wrap">
            <table className="data-table">
              <thead><tr><th>Project</th><th>Default branch</th><th>Index</th><th>Last analysis</th><th>Added</th></tr></thead>
              <tbody>{projects.map((project) => {
                const lastAnalysis = analyses.find((analysis) => analysis.project_id === project.id);
                const index = indexes.get(project.id);
                return <tr key={project.id}>
                  <td><Link href={`/projects/${project.id}`}><span className="table-title">{project.name}</span><span className="table-subtitle">{project.provider === "github" ? "GitHub" : "GitLab"} #{project.provider_project_id} · {project.language.toUpperCase()}</span></Link></td>
                  <td className="mono">{project.default_branch}</td>
                  <td><StatusBadge status={index?.status ?? "NOT_INDEXED"} /></td>
                  <td>{lastAnalysis ? <Link href={`/analyses/${lastAnalysis.id}`}><StatusBadge status={lastAnalysis.status} /></Link> : <span className="table-subtitle">No analysis yet</span>}</td>
                  <td>{formatDate(project.created_at)}</td>
                </tr>;
              })}</tbody>
            </table>
          </div>
        </section>
      ) : <EmptyState title="No projects connected" message="Create a project through the backend API, then use this workspace to inspect its indexing and review trail." />}
    </AppShell>
  );
}
