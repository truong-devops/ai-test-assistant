import Link from "next/link";
import { AppShell, EmptyState, PageHeading } from "@/components/shell";
import { Stat } from "@/components/stat";
import { StatusBadge } from "@/components/status-badge";
import { getAnalyses, getProjects, optional } from "@/lib/api";
import { formatDate, shortSHA } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function OverviewPage() {
  const [projects, analyses] = await Promise.all([
    optional(getProjects, []),
    optional(getAnalyses, []),
  ]);
  const waiting = analyses.filter((analysis) => analysis.status === "WAITING_REVIEW");
  const accepted = analyses.filter((analysis) => analysis.status === "ACCEPTED");
  const attention = analyses.filter((analysis) => ["FAILED", "REJECTED"].includes(analysis.status));

  return (
    <AppShell active="overview">
      <PageHeading
        eyebrow="Review control room"
        title="Evidence before approval."
        description="Follow each generated test from merge-request change to human decision, with the entire validation and repair record kept visible."
        actions={<Link className="button secondary" href="/analyses">Open review queue</Link>}
      />
      <div className="summary-grid">
        <Stat label="Connected projects" value={projects.length} detail="GitLab sources in this workspace" />
        <Stat label="Awaiting review" value={waiting.length} detail="Human decision required" tone={waiting.length ? "warning" : "plain"} />
        <Stat label="Accepted analyses" value={accepted.length} detail="All latest candidates accepted" tone={accepted.length ? "accent" : "plain"} />
        <Stat label="Needs attention" value={attention.length} detail="Failed or rejected outcomes" tone={attention.length ? "warning" : "plain"} />
      </div>
      <div className="content-grid">
        <section className="panel">
          <div className="panel-header">
            <div><h2>Recent analysis activity</h2><p>Latest merge-request analysis jobs across all projects.</p></div>
            <Link className="button secondary" href="/analyses">View all</Link>
          </div>
          {analyses.length ? (
            <div className="table-wrap">
              <table className="data-table">
                <thead><tr><th>Analysis</th><th>Source</th><th>Status</th><th>Created</th></tr></thead>
                <tbody>
                  {analyses.slice(0, 7).map((analysis) => (
                    <tr key={analysis.id}>
                      <td><Link href={`/analyses/${analysis.id}`}><span className="table-title">{analysis.title || `Merge request !${analysis.merge_request_iid}`}</span><span className="table-subtitle">MR !{analysis.merge_request_iid} · Project #{analysis.project_id}</span></Link></td>
                      <td className="mono">{shortSHA(analysis.source_sha)}</td>
                      <td><StatusBadge status={analysis.status} /></td>
                      <td>{formatDate(analysis.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : <div className="panel-body"><EmptyState title="No analysis jobs yet" message="Connect a GitLab project and send a merge-request webhook to begin building review evidence." /></div>}
        </section>
        <aside className="stack">
          <section className="panel side-section">
            <p className="eyebrow">Review discipline</p>
            <h2>Failures stay visible</h2>
            <p className="page-description">A passing sandbox result is evidence, not an automatic approval. Inspect the scenario, source context and generated assertions before accepting.</p>
          </section>
          <section className="panel side-section">
            <p className="eyebrow">Workflow</p>
            <h2>From change to decision</h2>
            <ol className="repair-list">
              <li className="repair-item"><span className="repair-dot">1</span><h4>Analyze</h4><p>Map changed Go symbols and project evidence.</p></li>
              <li className="repair-item"><span className="repair-dot">2</span><h4>Validate</h4><p>Run generated tests in an isolated sandbox.</p></li>
              <li className="repair-item"><span className="repair-dot">3</span><h4>Review</h4><p>Record the final human judgement.</p></li>
            </ol>
          </section>
        </aside>
      </div>
    </AppShell>
  );
}
