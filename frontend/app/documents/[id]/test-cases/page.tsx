import Link from "next/link";
import { notFound } from "next/navigation";
import { AppShell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { TestCaseWorkspace } from "@/components/test-case-workspace";
import { TestcaseGenerationWorkspace } from "@/components/testcase-generation-workspace";
import { ExportControls } from "@/components/export-controls";
import { SuiteReleasePublisher } from "@/components/suite-release-publisher";
import { DocumentWorkflowNav } from "@/components/document-workflow-nav";
import { ApiError, getBusinessTestCases, getCoverage, getDocumentSet, getDocumentWorkflow, getSuiteReleases, getTestExports, getRequirements, getAIBudget } from "@/lib/api";

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
  const [testCases, coverage, exports, releases, workflow] = await Promise.all([getBusinessTestCases(id), getCoverage(id), getTestExports(id), getSuiteReleases(id), getDocumentWorkflow(id)]);
  const requirements = await getRequirements(id);
  const budget = workflow.capabilities.can_manage ? await getAIBudget(id) : undefined;
  return <AppShell active="documents">
    <DocumentWorkflowNav setId={set.id} workflow={workflow} active="test-cases" />
    <div className="breadcrumb"><Link href="/documents">Documents</Link><span>/</span><Link href={`/documents/${id}`}>{set.name}</Link><span>/</span><span>Test cases</span></div>
    <div className="page-heading"><div><p className="eyebrow">Grounded test design</p><h1>Test cases and coverage</h1><p className="page-description">Generation creates proposals for review before changing working testcases. Conflict/TBD remains outside the approved denominator.</p></div></div>
    <TestcaseGenerationWorkspace key={set.id} setId={set.id} requirements={requirements} workflow={workflow} budget={budget} affectedRevisionCount={testCases.filter((c) => c.needs_source_review).length} />
    <div className="summary-grid"><article className="stat-card accent"><p>Working design coverage</p><strong>{coverage.baseline_complete ? `${coverage.coverage_percent.toFixed(1)}%` : `${coverage.covered_count}/${coverage.approved_denominator}`}</strong><small>{coverage.baseline_complete ? "Working design complete" : "Not complete while conflict/TBD/uncovered remains"}</small></article><article className="stat-card"><p>Uncovered</p><strong>{coverage.uncovered_count}</strong><small>Current approved requirements</small></article><article className="stat-card warning"><p>Outside denominator</p><strong>{coverage.conflict_count + coverage.tbd_count}</strong><small>{coverage.conflict_count} conflict · {coverage.tbd_count} TBD</small></article><article className="stat-card"><p>Duplicates suppressed</p><strong>{coverage.duplicate_count}</strong><small>Exact or semantic matches</small></article></div>
    <div className="summary-grid">{coverage.layers.map((layer) => <article className="stat-card" key={layer.key}><p>{layer.label}</p><strong>{layer.numerator}/{layer.denominator}</strong><small>{layer.denominator ? `${layer.percent.toFixed(1)}%` : "No denominator"}{layer.release_number ? ` · R${layer.release_number}` : " · working set"}{layer.source_snapshot_id ? ` · source #${layer.source_snapshot_id}` : ""}</small></article>)}</div>
	<TestCaseWorkspace setId={set.id} testCases={testCases} canReview={workflow.capabilities.can_review} />
	{testCases.length && workflow.capabilities.can_publish ? <SuiteReleasePublisher setId={set.id} suiteId={testCases[0].test_suite_id} testCases={testCases} releases={releases} /> : null}
	{testCases.length && workflow.capabilities.can_review ? <ExportControls setId={set.id} suiteId={testCases[0].test_suite_id} exports={exports} testCases={testCases} releases={releases} /> : null}
    <section className="document-preview-section panel"><div className="panel-header"><div><h2>Requirement ↔ test-case matrix</h2><p>Rejected test cases remain visible in the audit; unresolved source states are reported separately.</p></div><span className="section-counter">{coverage.matrix.length} requirements</span></div><div className="table-wrap"><table className="data-table"><thead><tr><th>Requirement</th><th>Flow</th><th>Source status</th><th>Test cases</th><th>Coverage</th><th>Warnings</th></tr></thead><tbody>{coverage.matrix.map((cell) => {
      const testCaseIds = cell.test_case_ids ?? [];
      const testTypes = cell.test_types ?? [];
      const warnings = cell.warnings ?? [];
      return <tr key={cell.requirement_id}><td><Link href={`/documents/${id}/requirements/${cell.requirement_id}`}><span className="table-title">{cell.requirement_key}</span><span className="table-subtitle">{cell.title}</span></Link></td><td><StatusBadge status={cell.flow_type} /></td><td><StatusBadge status={cell.status} /></td><td>{testCaseIds.length ? testCaseIds.map((caseId, index) => <span key={caseId}>{index ? ", " : ""}<Link href={`/documents/${id}/test-cases/${caseId}`}>#{caseId}</Link></span>) : "—"}<span className="table-subtitle">{testTypes.join(", ")}</span></td><td><StatusBadge status={cell.covered ? "COVERED" : "UNCOVERED"} /></td><td>{warnings.join(" · ") || "—"}</td></tr>;
    })}</tbody></table></div></section>
  </AppShell>;
}
