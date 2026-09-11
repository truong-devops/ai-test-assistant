import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { TestCaseWorkspace } from "@/components/test-case-workspace";
import { WorkflowAction } from "@/components/workflow-action";
import { ApiError, getBusinessTestCases, getCoverage, getDocumentSet } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function TestCasesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let set;
  try {
    set = await getDocumentSet(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const [testCases, coverage] = await Promise.all([getBusinessTestCases(id), getCoverage(id)]);
  return <AppShell active="documents">
    <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><span>Test cases</span></div>
    <div className="page-heading"><div><p className="eyebrow">Phase 5 · Grounded test design</p><h1>Test cases and coverage</h1><p className="page-description">Generation runs per approved requirement/flow. Coverage is rebuilt from database links and keeps conflict/TBD outside the approved denominator.</p></div><WorkflowAction endpoint={`/api/document-sets/${id}/test-cases/${testCases.length ? "regenerate" : "generate"}`} label={testCases.length ? "Regenerate" : "Generate test cases"} pendingLabel="Generating…" /></div>
    <div className="summary-grid"><article className="stat-card accent"><p>Approved coverage</p><strong>{coverage.baseline_complete ? `${coverage.coverage_percent.toFixed(1)}%` : `${coverage.covered_count}/${coverage.approved_denominator}`}</strong><small>{coverage.baseline_complete ? "Baseline complete" : "Not complete while conflict/TBD/uncovered remains"}</small></article><article className="stat-card"><p>Uncovered</p><strong>{coverage.uncovered_count}</strong><small>Approved baseline only</small></article><article className="stat-card warning"><p>Outside denominator</p><strong>{coverage.conflict_count + coverage.tbd_count}</strong><small>{coverage.conflict_count} conflict · {coverage.tbd_count} TBD</small></article><article className="stat-card"><p>Duplicates suppressed</p><strong>{coverage.duplicate_count}</strong><small>Exact or semantic matches</small></article></div>
    {testCases.length ? <TestCaseWorkspace setId={set.id} testCases={testCases} /> : <EmptyState title="No business test cases" message="Approve at least one cited requirement, then generate the test baseline." />}
    <section className="document-preview-section panel"><div className="panel-header"><div><h2>Requirement ↔ test-case matrix</h2><p>Rejected test cases remain visible in the audit; unresolved source states are reported separately.</p></div><span className="section-counter">{coverage.matrix.length} requirements</span></div><div className="table-wrap"><table className="data-table"><thead><tr><th>Requirement</th><th>Flow</th><th>Source status</th><th>Test cases</th><th>Coverage</th><th>Warnings</th></tr></thead><tbody>{coverage.matrix.map((cell) => {
      const testCaseIds = cell.test_case_ids ?? [];
      const testTypes = cell.test_types ?? [];
      const warnings = cell.warnings ?? [];
      return <tr key={cell.requirement_id}><td><Link href={`/documents/${id}/requirements/${cell.requirement_id}`}><span className="table-title">{cell.requirement_key}</span><span className="table-subtitle">{cell.title}</span></Link></td><td><StatusBadge status={cell.flow_type} /></td><td><StatusBadge status={cell.status} /></td><td>{testCaseIds.length ? testCaseIds.map((caseId, index) => <span key={caseId}>{index ? ", " : ""}<Link href={`/documents/${id}/test-cases/${caseId}`}>#{caseId}</Link></span>) : "—"}<span className="table-subtitle">{testTypes.join(", ")}</span></td><td><StatusBadge status={cell.covered ? "COVERED" : "UNCOVERED"} /></td><td>{warnings.join(" · ") || "—"}</td></tr>;
    })}</tbody></table></div></section>
  </AppShell>;
}
