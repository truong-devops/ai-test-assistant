import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell } from "@/components/shell";
import { Stat } from "@/components/stat";
import { ApiError, getEvaluation } from "@/lib/api";
import type { RateMetric } from "@/lib/types";

export const dynamic = "force-dynamic";

const labels: Record<string, string> = {
  CONTEXT_IMPACT: "A · Context impact", REPAIR_IMPACT: "B · Repair impact", HUMAN_EFFORT: "C · Human effort",
  DIFF_ONLY: "Diff only", DIFF_RAG: "Diff + RAG", GENERATE_ONLY: "Generate only",
  GENERATE_REPAIR: "Generate + repair", MANUAL: "Manual", AI_ASSISTED: "AI-assisted",
};
const rate = (value?: RateMetric) => value ? `${value.rate_pct.toFixed(2)}% (${value.successes}/${value.total})` : "—";
const delta = (value?: number, suffix = "pp") => value === undefined ? "—" : `${value >= 0 ? "+" : ""}${value.toFixed(2)} ${suffix}`;

export default async function EvaluationDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let stored;
  try { stored = await getEvaluation(id); } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const { run, report } = stored;
  const context = report.comparisons.find((item) => item.experiment === "CONTEXT_IMPACT");
  const repair = report.comparisons.find((item) => item.experiment === "REPAIR_IMPACT");
  const effort = report.comparisons.find((item) => item.experiment === "HUMAN_EFFORT");
  return <AppShell active="evaluations">
    <div className="breadcrumb"><Link href="/evaluations">Evaluation runs</Link><span>/</span><span>#{run.id}</span></div>
    <section className="project-hero"><div className="hero-identity"><span className="project-avatar evaluation-avatar" aria-hidden="true">E</span><div><p className="eyebrow">{report.schema_version}</p><h1>{report.dataset_name}</h1><p>{report.description}</p></div></div><div className="hero-meta"><span className="status-badge positive">Reproducible</span></div></section>
    <section className="panel"><div className="detail-grid"><div className="detail-cell"><span>Observations</span><strong>{run.observation_count}</strong></div><div className="detail-cell"><span>Dataset hash</span><strong className="mono evaluation-hash">{report.dataset_hash}</strong></div><div className="detail-cell"><span>Comparison design</span><strong>Paired by scenario + replicate</strong></div></div></section>
    <div className="summary-grid evaluation-summary">
      <Stat label="Context · execution" value={delta(context?.execution_validity_delta_pp)} detail="Diff + RAG vs diff only" />
      <Stat label="Repair · final success" value={delta(repair?.final_success_delta_pp)} detail={`Repair success ${repair?.repair_success_rate_pct?.toFixed(2) ?? "—"}%`} />
      <Stat label="Human effort saved" value={delta(effort?.mean_duration_reduction_pct, "%")} detail={`${effort?.mean_duration_reduction_seconds?.toFixed(0) ?? "—"} seconds mean reduction`} />
      <Stat label="Coverage contribution" value={delta(context?.mean_coverage_delta_change_pp)} detail="Treatment change vs baseline" />
    </div>
    <section className="panel">
      <div className="panel-header"><div><h2>Metric ledger</h2><p>Missing metrics are shown as — and never counted in a denominator.</p></div></div>
      <div className="table-wrap"><table className="data-table"><thead><tr><th>Experiment / variant</th><th>N</th><th>Syntax</th><th>Compile</th><th>Execution</th><th>Human accepted</th><th>First pass</th><th>Repair</th><th>Final</th><th>Coverage Δ</th><th>Time</th></tr></thead>
        <tbody>{report.groups.map((group) => <tr key={`${group.experiment}-${group.variant}`}><td><span className="table-title">{labels[group.variant] ?? group.variant}</span><span className="table-subtitle">{labels[group.experiment] ?? group.experiment}</span></td><td>{group.observation_count}</td><td>{rate(group.syntactic_validity)}</td><td>{rate(group.compile_validity)}</td><td>{rate(group.execution_validity)}</td><td>{rate(group.human_acceptance)}</td><td>{rate(group.first_pass_success)}</td><td>{rate(group.repair_success)}</td><td>{rate(group.final_success)}</td><td>{group.mean_coverage_delta_pp === undefined ? "—" : `${group.mean_coverage_delta_pp.toFixed(2)} pp`}</td><td>{group.mean_duration_seconds === undefined ? "—" : `${group.mean_duration_seconds.toFixed(0)}s`}</td></tr>)}</tbody>
      </table></div>
    </section>
  </AppShell>;
}
