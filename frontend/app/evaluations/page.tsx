import Link from "next/link";
import { AppShell, EmptyState, PageHeading } from "@/components/shell";
import { getEvaluations, optional } from "@/lib/api";
import { formatDate } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function EvaluationsPage() {
  const runs = await optional(getEvaluations, []);
  return (
    <AppShell active="evaluations">
      <PageHeading eyebrow="Phase 10 · Thesis evidence" title="Evaluation runs" description="Immutable experiment datasets with paired baselines, explicit denominators and reproducible report artifacts." />
      <div className="notice evaluation-notice"><strong>Interpretation guardrail</strong>A passing test is not automatically useful. Syntax, compile, execution, coverage and human acceptance remain separate.</div>
      {runs.length ? <section className="panel evaluation-runs">
        <div className="panel-header"><div><h2>Recorded datasets</h2><p>Identical dataset hashes are imported idempotently.</p></div><span className="section-counter">{runs.length} run{runs.length === 1 ? "" : "s"}</span></div>
        <div className="table-wrap"><table className="data-table"><thead><tr><th>Dataset</th><th>Observations</th><th>SHA-256</th><th>Recorded</th></tr></thead>
          <tbody>{runs.map((run) => <tr key={run.id}><td><Link href={`/evaluations/${run.id}`}><span className="table-title">{run.name}</span><span className="table-subtitle">{run.description}</span></Link></td><td>{run.observation_count}</td><td className="mono hash-cell">{run.dataset_hash.slice(0, 12)}…</td><td>{formatDate(run.created_at)}</td></tr>)}</tbody>
        </table></div>
      </section> : <EmptyState title="No evaluation run imported" message="Run make evaluate-import after migrations. Offline JSON, CSV, Markdown and SVG artifacts can still be generated with make evaluate." />}
    </AppShell>
  );
}
