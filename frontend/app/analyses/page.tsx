import Link from "next/link";
import { AppShell, EmptyState, PageHeading } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { getAnalyses, optional } from "@/lib/api";
import { formatDate, shortSHA } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function AnalysesPage() {
  const analyses = await optional(getAnalyses, []);
  const reviewReady = analyses.filter((analysis) => analysis.status === "WAITING_REVIEW");
  return (
    <AppShell active="analyses">
      <PageHeading eyebrow="Human decision queue" title="Analysis reviews" description="Open an analysis to inspect the change, RAG evidence, generated tests and every validation or repair attempt." actions={<span className="status-badge attention">{reviewReady.length} awaiting review</span>} />
      {analyses.length ? <section className="panel"><div className="panel-header"><div><h2>All analysis jobs</h2><p>Latest first. Failed attempts are retained as review evidence.</p></div></div><div className="table-wrap"><table className="data-table"><thead><tr><th>Merge request</th><th>Project</th><th>Commit</th><th>Status</th><th>Created</th></tr></thead><tbody>{analyses.map((analysis) => <tr key={analysis.id}><td><Link href={`/analyses/${analysis.id}`}><span className="table-title">{analysis.title || `Merge request !${analysis.merge_request_iid}`}</span><span className="table-subtitle">!{analysis.merge_request_iid} · {analysis.source_branch || "unknown source"} → {analysis.target_branch || "unknown target"}</span></Link></td><td>#{analysis.project_id}</td><td className="mono">{shortSHA(analysis.source_sha)}</td><td><StatusBadge status={analysis.status} /></td><td>{formatDate(analysis.created_at)}</td></tr>)}</tbody></table></div></section> : <EmptyState title="The review queue is empty" message="Analysis jobs appear after a supported GitLab merge-request webhook is processed." />}
    </AppShell>
  );
}
