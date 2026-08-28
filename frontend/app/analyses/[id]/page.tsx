import Link from "next/link";
import { notFound } from "next/navigation";
import { CodeBlock } from "@/components/code-block";
import { ReviewDecision } from "@/components/review-decision";
import { AppShell, EmptyState } from "@/components/shell";
import { Stat } from "@/components/stat";
import { StatusBadge } from "@/components/status-badge";
import {
  ApiError,
  getAnalysis,
  getContext,
  getGeneratedTests,
  getRecommendations,
  getRepairs,
  getReviews,
  getValidations,
  optional,
} from "@/lib/api";
import { formatDate, formatDuration, latestGeneratedTests, shortSHA } from "@/lib/presentation";
import type { GeneratedTest, Recommendation, RepairAttempt, Review, ValidationRun } from "@/lib/types";

export const dynamic = "force-dynamic";

function ValidationEvidence({
  runs,
  generatedByID,
}: {
  runs: ValidationRun[];
  generatedByID: Map<number, GeneratedTest>;
}) {
  if (!runs.length) return <p className="table-subtitle">No sandbox validation run has been stored yet.</p>;
  return <div className="validation-list">{runs.map((run) => {
    const version = generatedByID.get(run.generated_test_id)?.generation_attempt;
    const output = [run.stderr, run.stdout].filter(Boolean).join("\n\n") || "The sandbox did not emit stdout or stderr.";
    return <details className="validation-row" key={run.id} open={run.status !== "PASSED"}>
      <summary>
        <span className="validation-label"><StatusBadge status={run.status} /><strong>Validation {run.attempt_number}</strong><span>candidate v{version ?? "?"}</span></span>
        <span className="validation-meta">exit {run.exit_code} · {formatDuration(run.duration_ms)}</span>
      </summary>
      <div className="validation-output">
        <div className="validation-command">{run.command}</div>
        <CodeBlock title={run.output_truncated ? "Sandbox output · truncated" : "Sandbox output"} code={output} label="LOG" compact />
      </div>
    </details>;
  })}</div>;
}

function RepairEvidence({ attempts }: { attempts: RepairAttempt[] }) {
  if (!attempts.length) return <p className="table-subtitle">No repair was needed for this candidate.</p>;
  return <ol className="repair-list">{attempts.map((attempt) => <li className="repair-item" key={attempt.id}>
    <span className="repair-dot">{attempt.attempt_number}</span>
    <h4>Repair attempt {attempt.attempt_number}</h4>
    <p>{attempt.reason}</p>
    <small>{formatDate(attempt.created_at)} · {attempt.model_name} · {attempt.prompt_version}</small>
    <details>
      <summary>Inspect before / after test source</summary>
      <div className="repair-codes">
        <CodeBlock title="Previous generated test" code={attempt.previous_code} label="GO" compact />
        <CodeBlock title="Repaired generated test" code={attempt.repaired_code} label="GO" compact />
      </div>
    </details>
  </li>)}</ol>;
}

function CandidateCard({
  candidate,
  ordinal,
  analysisStatus,
  recommendation,
  allGenerated,
  generatedByID,
  validations,
  repairs,
  review,
}: {
  candidate: GeneratedTest;
  ordinal: number;
  analysisStatus: string;
  recommendation?: Recommendation;
  allGenerated: GeneratedTest[];
  generatedByID: Map<number, GeneratedTest>;
  validations: ValidationRun[];
  repairs: RepairAttempt[];
  review?: Review;
}) {
  const candidateIDs = new Set(allGenerated.filter((item) => item.recommendation_id === candidate.recommendation_id).map((item) => item.id));
  const relatedValidations = validations.filter((run) => candidateIDs.has(run.generated_test_id));
  const relatedRepairs = repairs.filter((attempt) => candidateIDs.has(attempt.generated_test_id) || candidateIDs.has(attempt.repaired_generated_test_id));
  return <article className="candidate-card">
    <header className="candidate-header">
      <div className="candidate-title">
        <span className="candidate-number">{String(ordinal).padStart(2, "0")}</span>
        <div>
          <h3>{recommendation?.title ?? `Generated candidate #${candidate.id}`}</h3>
          <p>{recommendation?.description ?? "No recommendation record is available for this candidate."}</p>
          <span className="candidate-path">{candidate.file_path} · version {candidate.generation_attempt}</span>
        </div>
      </div>
      <StatusBadge status={review?.decision ?? analysisStatus} />
    </header>
    <div className="candidate-body">
      {recommendation ? <>
        <div className="scenario-grid">
          <div className="scenario-item"><span>Recommended scenario</span><p>{recommendation.scenario}</p></div>
          <div className="scenario-item"><span>Expected behaviour</span><p>{recommendation.expected_behavior}</p></div>
        </div>
        <div className="rationale"><strong>Why this was suggested.</strong> {recommendation.rationale}</div>
      </> : null}
      <CodeBlock title={`Generated test · ${candidate.file_path}`} code={candidate.code} label="GO" />
      <section>
        <div className="section-heading"><div><h2>Validation evidence</h2><p>All versions are retained; failures are not hidden.</p></div><span className="section-counter">{relatedValidations.length} run{relatedValidations.length === 1 ? "" : "s"}</span></div>
        <ValidationEvidence runs={relatedValidations} generatedByID={generatedByID} />
      </section>
      <section>
        <div className="section-heading"><div><h2>Repair history</h2><p>Every replacement remains linked to its failed source.</p></div><span className="section-counter">{relatedRepairs.length} attempt{relatedRepairs.length === 1 ? "" : "s"}</span></div>
        <RepairEvidence attempts={relatedRepairs} />
      </section>
      <ReviewDecision generatedTestId={candidate.id} enabled={analysisStatus === "WAITING_REVIEW"} existing={review} />
    </div>
  </article>;
}

export default async function AnalysisReviewPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let detail;
  try {
    detail = await getAnalysis(id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  const [recommendations, generatedTests, validations, repairs, reviews, context] = await Promise.all([
    optional<Recommendation[]>(() => getRecommendations(id), []),
    optional<GeneratedTest[]>(() => getGeneratedTests(id), []),
    optional<ValidationRun[]>(() => getValidations(id), []),
    optional<RepairAttempt[]>(() => getRepairs(id), []),
    optional<Review[]>(() => getReviews(id), []),
    optional(() => getContext(id), []),
  ]);
  const { analysis, changed_files: changedFiles, changed_symbols: changedSymbols } = detail;
  const latestCandidates = latestGeneratedTests(generatedTests);
  const recommendationsByID = new Map(recommendations.map((item) => [item.id, item]));
  const generatedByID = new Map(generatedTests.map((item) => [item.id, item]));
  const reviewsByGeneratedID = new Map(reviews.map((item) => [item.generated_test_id, item]));
  const passed = validations.filter((run) => run.status === "PASSED").length;

  return <AppShell active="analyses">
    <div className="breadcrumb"><Link href="/analyses">Review queue</Link><span>/</span><span>Analysis #{analysis.id}</span></div>
    <section className="review-hero">
      <div className="review-hero-top">
        <div><p className="eyebrow">Merge request !{analysis.merge_request_iid} · Project #{analysis.project_id}</p><h1>{analysis.title || "Untitled merge request"}</h1><p>{analysis.source_branch || "source"} → {analysis.target_branch || "target"} · created {formatDate(analysis.created_at)}</p></div>
        <StatusBadge status={analysis.status} />
      </div>
      <div className="review-facts">
        <div className="review-fact"><span>Source commit</span><strong className="mono">{shortSHA(analysis.source_sha)}</strong></div>
        <div className="review-fact"><span>Target commit</span><strong className="mono">{shortSHA(analysis.target_sha)}</strong></div>
        <div className="review-fact"><span>Review candidates</span><strong>{latestCandidates.length}</strong></div>
        <div className="review-fact"><span>Final decisions</span><strong>{reviews.length} / {latestCandidates.length}</strong></div>
      </div>
    </section>
    {analysis.error_message ? <p className="notice"><strong>Pipeline note</strong>{analysis.error_message}</p> : null}
    <div className="review-layout">
      <div className="review-main">
        <section>
          <div className="section-heading"><div><h2>Changed source</h2><p>Raw merge-request evidence used to map the review scope.</p></div><span className="section-counter">{changedFiles.length} file{changedFiles.length === 1 ? "" : "s"}</span></div>
          {changedFiles.length ? <div className="change-list">{changedFiles.map((file) => <article className="change-card" key={file.id}><header><strong>{file.new_path || file.old_path}</strong><span className="change-metrics"><b className="added">+{file.additions}</b><b className="removed">−{file.deletions}</b></span></header><CodeBlock title="Merge-request diff" code={file.diff || "No diff content was retained for this file."} label="DIFF" compact /></article>)}</div> : <EmptyState title="No changed files recorded" message="This analysis has not yet produced a persisted source-change record." />}
        </section>
        <section>
          <div className="section-heading"><div><h2>Generated test review</h2><p>Inspect each current candidate alongside its full version and validation trail.</p></div><span className="section-counter">{latestCandidates.length} current</span></div>
          {latestCandidates.length ? <div className="candidate-list">{latestCandidates.map((candidate, index) => <CandidateCard key={candidate.id} candidate={candidate} ordinal={index + 1} analysisStatus={analysis.status} recommendation={recommendationsByID.get(candidate.recommendation_id)} allGenerated={generatedTests} generatedByID={generatedByID} validations={validations} repairs={repairs} review={reviewsByGeneratedID.get(candidate.id)} />)}</div> : <EmptyState title="No generated test candidates" message="Generated tests will appear here after recommendation and generation complete." />}
        </section>
      </div>
      <aside className="review-sidebar">
        <section className="panel side-section">
          <p className="eyebrow">Sandbox summary</p><h2>Validation signal</h2>
          <div className="summary-grid" style={{ gridTemplateColumns: "1fr 1fr", margin: "14px 0 0" }}><Stat label="Passes" value={passed} detail="Stored sandbox runs" /><Stat label="Repairs" value={repairs.length} detail="Bounded attempts" /></div>
        </section>
        <section className="panel side-section">
          <p className="eyebrow">Changed symbols</p><h2>Review scope</h2>
          {changedSymbols.length ? <div className="symbol-list">{changedSymbols.map((symbol) => <div className="symbol-row" key={symbol.id}><div><strong>{symbol.receiver_name ? `${symbol.receiver_name}.` : ""}{symbol.symbol_name}</strong><p>{symbol.change_summary}</p></div><span>{symbol.package_name}:{symbol.start_line}–{symbol.end_line}</span></div>)}</div> : <p className="table-subtitle">No Go symbols were mapped from this change.</p>}
        </section>
        <section className="panel side-section">
          <p className="eyebrow">Retrieved context</p><h2>Project evidence</h2>
          <p className="page-description">Current project-index retrieval, grouped by source path. Open a record to inspect its exact content.</p>
          {context.length ? <div className="evidence-list" style={{ marginTop: 14 }}>{context.map((chunk) => <details className="evidence-card" key={chunk.id || chunk.chunk_key}><summary><div><strong>{chunk.file_path}</strong><span>{chunk.symbol_name || chunk.package_name || "project context"} · lines {chunk.start_line}–{chunk.end_line}</span></div><small>{chunk.chunk_type}</small></summary><pre className="evidence-content">{chunk.content}</pre></details>)}</div> : <p className="table-subtitle" style={{ marginTop: 12 }}>No indexed context is available. Request a project re-index, then refresh this review.</p>}
        </section>
      </aside>
    </div>
  </AppShell>;
}
