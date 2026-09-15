import type {
  Analysis,
  AnalysisDetail,
  GeneratedTest,
  IndexStatus,
  KnowledgeChunk,
  Project,
  Recommendation,
  RepairAttempt,
  Review,
  ValidationRun,
  EvaluationRun,
  StoredEvaluation,
  ProvenanceBundle,
  ImpactBundle,
  DocumentSet,
  DocumentPipelineMetrics,
  SourceDocument,
  DocumentVersionDetail,
  DocumentIndexStatus,
  SemanticChunk,
  Requirement,
  RequirementExtractionJob,
  RequirementDetail,
  RequirementConflict,
  OpenQuestion,
  BusinessTestCase,
  BusinessTestCaseDetail,
  CoverageReport,
  TestExport,
  BaselineView,
  AnalysisTestScope,
  AutomationHistory,
	TestRunDetail,
} from "@/lib/types";
import { backendAuthHeaders } from "@/lib/backend-auth";

const backendOrigin = process.env.BACKEND_API_URL?.replace(/\/$/, "") ?? "http://localhost:8080";

const configuredDocumentMaxBytes = Number(process.env.DOCUMENT_MAX_UPLOAD_BYTES ?? 16 * 1024 * 1024);
export const documentMaxUploadBytes = Number.isSafeInteger(configuredDocumentMaxBytes) &&
  configuredDocumentMaxBytes > 0 && configuredDocumentMaxBytes <= 256 * 1024 * 1024
  ? configuredDocumentMaxBytes
  : 16 * 1024 * 1024;

export const documentRoutes = {
  sets: "/api/document-sets",
  set: (id: string | number) => `/api/document-sets/${encodeURIComponent(String(id))}`,
  documents: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/documents`,
  version: (documentId: string | number, version: string | number) =>
    `/api/documents/${encodeURIComponent(String(documentId))}/versions/${encodeURIComponent(String(version))}`,
  index: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/index`,
  chunks: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/chunks`,
  requirements: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/requirements`,
  requirementExtraction: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/requirements/extraction`,
  requirement: (id: string | number) => `/api/requirements/${encodeURIComponent(String(id))}`,
  conflicts: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/requirement-conflicts`,
  questions: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/open-questions`,
  testCases: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/test-cases`,
  testCase: (id: string | number) => `/api/test-cases/${encodeURIComponent(String(id))}`,
  coverage: (setId: string | number) => `/api/document-sets/${encodeURIComponent(String(setId))}/coverage`,
} as const;

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${backendOrigin}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json", ...backendAuthHeaders() },
  });
  if (!response.ok) {
    let message = `Request failed with HTTP ${response.status}`;
    try {
      const body = (await response.json()) as { error?: string };
      message = body.error ?? message;
    } catch {
      // Keep the stable status message when an upstream proxy returns non-JSON.
    }
    throw new ApiError(message, response.status);
  }
  return (await response.json()) as T;
}

export async function optional<T>(load: () => Promise<T>, fallback: T): Promise<T> {
  try {
    return await load();
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return fallback;
    }
    throw error;
  }
}

export async function getProjects(): Promise<Project[]> {
  return (await request<{ projects: Project[] }>("/api/projects")).projects;
}

export async function getDocumentSets(): Promise<DocumentSet[]> {
  return (await request<{ document_sets: DocumentSet[] }>(documentRoutes.sets)).document_sets;
}

export async function getDocumentPipelineMetrics(): Promise<DocumentPipelineMetrics> {
  return request<DocumentPipelineMetrics>("/api/document-metrics");
}

export async function getDocumentSet(id: string | number): Promise<DocumentSet> {
  return request<DocumentSet>(documentRoutes.set(id));
}

export async function getDocuments(setId: string | number): Promise<SourceDocument[]> {
  return (await request<{ documents: SourceDocument[] }>(
    documentRoutes.documents(setId),
  )).documents;
}

export async function getDocumentVersion(documentId: string | number, version: string | number): Promise<DocumentVersionDetail> {
  return request<DocumentVersionDetail>(
    documentRoutes.version(documentId, version),
  );
}

export async function getDocumentIndex(setId: string | number): Promise<DocumentIndexStatus> {
  return (await request<{ index: DocumentIndexStatus }>(documentRoutes.index(setId))).index;
}

export async function getDocumentChunks(setId: string | number): Promise<SemanticChunk[]> {
  return (await request<{ chunks: SemanticChunk[] }>(documentRoutes.chunks(setId))).chunks;
}

export async function getRequirements(setId: string | number): Promise<Requirement[]> {
  return (await request<{ requirements: Requirement[] }>(documentRoutes.requirements(setId))).requirements;
}

export async function getRequirementExtraction(setId: string | number): Promise<RequirementExtractionJob> {
  return (await request<{ job: RequirementExtractionJob }>(
    documentRoutes.requirementExtraction(setId),
  )).job;
}

export async function getRequirement(id: string | number): Promise<RequirementDetail> {
  return request<RequirementDetail>(documentRoutes.requirement(id));
}

export async function getRequirementConflicts(setId: string | number): Promise<RequirementConflict[]> {
  return (await request<{ conflicts: RequirementConflict[] }>(documentRoutes.conflicts(setId))).conflicts;
}

export async function getOpenQuestions(setId: string | number): Promise<OpenQuestion[]> {
  return (await request<{ open_questions: OpenQuestion[] }>(documentRoutes.questions(setId))).open_questions;
}

export async function getBusinessTestCases(setId: string | number): Promise<BusinessTestCase[]> {
  return (await request<{ test_cases: BusinessTestCase[] }>(documentRoutes.testCases(setId))).test_cases;
}

export async function getBusinessTestCase(id: string | number): Promise<BusinessTestCaseDetail> {
  return request<BusinessTestCaseDetail>(documentRoutes.testCase(id));
}

export async function getCoverage(setId: string | number): Promise<CoverageReport> {
  return request<CoverageReport>(documentRoutes.coverage(setId));
}

export async function getTestExports(setId: string | number): Promise<TestExport[]> {
  return (await request<{ exports: TestExport[] }>(`/api/document-sets/${encodeURIComponent(String(setId))}/exports`)).exports;
}

export async function getProjectBaseline(projectId: string | number): Promise<BaselineView> {
  return request<BaselineView>(`/api/projects/${encodeURIComponent(String(projectId))}/document-baseline`);
}

export async function getAnalysisTestScope(analysisId: string | number): Promise<AnalysisTestScope> {
  return request<AnalysisTestScope>(`/api/analyses/${encodeURIComponent(String(analysisId))}/test-scope`);
}

export async function getAutomationHistory(testCaseId: string | number): Promise<AutomationHistory> {
  return request<AutomationHistory>(`/api/test-cases/${encodeURIComponent(String(testCaseId))}/automation`);
}

export async function getTestRun(testRunId: string | number): Promise<TestRunDetail> {
	return request<TestRunDetail>(`/api/test-runs/${encodeURIComponent(String(testRunId))}`);
}

export async function getProject(id: string): Promise<Project> {
  return request<Project>(`/api/projects/${encodeURIComponent(id)}`);
}

export async function getIndexStatus(id: string | number): Promise<IndexStatus> {
  return (await request<{ index: IndexStatus }>(`/api/projects/${encodeURIComponent(String(id))}/index/status`)).index;
}

export async function getAnalyses(): Promise<Analysis[]> {
  return (await request<{ analyses: Analysis[] }>("/api/analyses")).analyses;
}

export async function getAnalysis(id: string): Promise<AnalysisDetail> {
  return request<AnalysisDetail>(`/api/analyses/${encodeURIComponent(id)}`);
}

export async function getRecommendations(id: string): Promise<Recommendation[]> {
  return (await request<{ recommendations: Recommendation[] }>(`/api/analyses/${encodeURIComponent(id)}/recommendations`)).recommendations;
}

export async function getGeneratedTests(id: string): Promise<GeneratedTest[]> {
  return (await request<{ generated_tests: GeneratedTest[] }>(`/api/analyses/${encodeURIComponent(id)}/generated-tests`)).generated_tests;
}

export async function getValidations(id: string): Promise<ValidationRun[]> {
  return (await request<{ validation_runs: ValidationRun[] }>(`/api/analyses/${encodeURIComponent(id)}/validations`)).validation_runs;
}

export async function getRepairs(id: string): Promise<RepairAttempt[]> {
  return (await request<{ repair_attempts: RepairAttempt[] }>(`/api/analyses/${encodeURIComponent(id)}/repairs`)).repair_attempts;
}

export async function getReviews(id: string): Promise<Review[]> {
  return (await request<{ reviews: Review[] }>(`/api/analyses/${encodeURIComponent(id)}/reviews`)).reviews;
}

export async function getContext(id: string): Promise<KnowledgeChunk[]> {
  return (await request<{ context: KnowledgeChunk[] }>(`/api/analyses/${encodeURIComponent(id)}/context`)).context;
}

export async function getEvaluations(): Promise<EvaluationRun[]> {
  return (await request<{ evaluation_runs: EvaluationRun[] }>("/api/evaluations")).evaluation_runs;
}

export async function getEvaluation(id: string): Promise<StoredEvaluation> {
  return request<StoredEvaluation>(`/api/evaluations/${encodeURIComponent(id)}`);
}

export async function getEvidence(id: string): Promise<ProvenanceBundle> {
  return (await request<{ evidence: ProvenanceBundle }>(`/api/analyses/${encodeURIComponent(id)}/evidence`)).evidence;
}

export async function getImpact(id: string): Promise<ImpactBundle> {
  return (await request<{ impact: ImpactBundle }>(`/api/analyses/${encodeURIComponent(id)}/impact`)).impact;
}
