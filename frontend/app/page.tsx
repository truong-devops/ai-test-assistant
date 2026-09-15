import Link from "next/link";
import { AppShell, EmptyState, PageHeading } from "@/components/shell";
import { Stat } from "@/components/stat";
import { StatusBadge } from "@/components/status-badge";
import { getAnalyses, getDocumentPipelineMetrics, optional } from "@/lib/api";
import { formatDate, shortSHA } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function OverviewPage() {
  const [metrics, analyses] = await Promise.all([
    optional(getDocumentPipelineMetrics, {
      document_sets: 0, document_versions: 0, parsed_versions: 0, parse_failures: 0,
      approved_requirements: 0, extraction_jobs: 0, extraction_failures: 0,
      test_suites: 0, approved_test_cases: 0, automation_artifacts: 0,
      test_runs: 0, runs_needing_attention: 0, pending_approval_actions: 0,
    }),
    optional(getAnalyses, []),
  ]);

  return (
    <AppShell active="overview">
      <PageHeading
        eyebrow="Workspace overview"
        title="Document-driven testing"
        description="Build an approved business baseline from documents, generate traceable test cases, then run approved automation against pull-request changes."
        actions={<Link className="button" href="/documents">Open documents</Link>}
      />
      <div className="summary-grid">
        <Stat label="Documents" value={metrics.document_versions} detail={`${metrics.parsed_versions} parsed · ${metrics.parse_failures} failed`} />
        <Stat label="Approved requirements" value={metrics.approved_requirements} detail={`${metrics.pending_approval_actions} approval action(s) pending`} tone={metrics.pending_approval_actions ? "warning" : "plain"} />
        <Stat label="Test suites / cases" value={`${metrics.test_suites} / ${metrics.approved_test_cases}`} detail={`${metrics.automation_artifacts} automation artifact(s)`} tone={metrics.approved_test_cases ? "accent" : "plain"} />
        <Stat label="Sandbox runs" value={metrics.test_runs} detail={`${metrics.runs_needing_attention} need attention`} tone={metrics.runs_needing_attention ? "warning" : "plain"} />
      </div>
      <div className="content-grid">
        <section className="panel">
          <div className="panel-header">
            <div><h2>Recent code-change runs</h2><p>Pull-request changes use the approved document test baseline; code is technical execution context only.</p></div>
            <Link className="button secondary" href="/analyses">View change runs</Link>
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
          ) : <div className="panel-body"><EmptyState title="No change runs yet" message="Approve a document-derived suite, bind it to a project, then send a GitHub or GitLab webhook." action={<Link className="button" href="/documents">Create document baseline</Link>} /></div>}
        </section>
        <aside className="stack">
          <section className="panel side-section">
            <p className="eyebrow">Source of truth</p>
            <h2>Business meaning comes from documents</h2>
            <p className="page-description">Repository code may help implement and execute automation, but it cannot create or change the approved requirement, expected result, or semantic assertions.</p>
          </section>
          <section className="panel side-section">
            <p className="eyebrow">Workflow</p>
            <h2>From evidence to report</h2>
            <ol className="repair-list">
              <li className="repair-item"><span className="repair-dot">1</span><h4>Approve evidence</h4><p>Parse documents, extract requirements and approve the business baseline.</p></li>
              <li className="repair-item"><span className="repair-dot">2</span><h4>Design tests</h4><p>Generate cited test cases and review coverage before automation.</p></li>
              <li className="repair-item"><span className="repair-dot">3</span><h4>Execute changes</h4><p>Use PR impact to run approved artifacts in the sandbox and export XLSX evidence.</p></li>
            </ol>
          </section>
        </aside>
      </div>
    </AppShell>
  );
}
