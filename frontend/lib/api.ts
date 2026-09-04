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
} from "@/lib/types";

const backendOrigin = process.env.BACKEND_API_URL?.replace(/\/$/, "") ?? "http://localhost:8080";

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
    headers: { Accept: "application/json" },
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
