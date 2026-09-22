package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/project"
)

func NewRouter(logger *slog.Logger, checker ReadinessChecker, projectService *project.Service,
	analysisService AnalysisService, webhookHandler http.Handler) http.Handler {
	return NewRouterWithAllServices(logger, checker, projectService, analysisService, webhookHandler, nil, nil, nil)
}

func NewRouterWithKnowledge(logger *slog.Logger, checker ReadinessChecker, projectService *project.Service,
	analysisService AnalysisService, webhookHandler http.Handler, knowledgeService KnowledgeService) http.Handler {
	return NewRouterWithAllServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, nil, nil)
}

func NewRouterWithServices(logger *slog.Logger, checker ReadinessChecker, projectService *project.Service,
	analysisService AnalysisService, webhookHandler http.Handler, knowledgeService KnowledgeService,
	recommendationService RecommendationService,
) http.Handler {
	return NewRouterWithAllServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, recommendationService, nil)
}

func NewRouterWithAllServices(logger *slog.Logger, checker ReadinessChecker, projectService *project.Service,
	analysisService AnalysisService, webhookHandler http.Handler, knowledgeService KnowledgeService,
	recommendationService RecommendationService, generationService GenerationService,
) http.Handler {
	return NewRouterWithPhaseSevenServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService, nil)
}

func NewRouterWithPhaseSevenServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
) http.Handler {
	return NewRouterWithPhaseEightServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService,
		validationService, nil)
}

func NewRouterWithPhaseEightServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService,
) http.Handler {
	return NewRouterWithPhaseNineServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService,
		validationService, repairService, nil, nil)
}

func NewRouterWithPhaseNineServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
) http.Handler {
	return NewRouterWithPhaseTenServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService,
		validationService, repairService, reviewService, contextService, nil)
}

func NewRouterWithPhaseTenServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService,
) http.Handler {
	return NewRouterWithPhaseElevenServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService,
		validationService, repairService, reviewService, contextService, evaluationService, RouterOptions{})
}

func NewRouterWithPhaseElevenServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, options RouterOptions,
) http.Handler {
	return NewRouterWithPhaseTwelveServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService,
		validationService, repairService, reviewService, contextService, evaluationService,
		nil, options)
}

func NewRouterWithPhaseTwelveServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService, options RouterOptions,
) http.Handler {
	return NewRouterWithPhaseThirteenServices(logger, checker, projectService, analysisService,
		webhookHandler, knowledgeService, recommendationService, generationService,
		validationService, repairService, reviewService, contextService, evaluationService,
		provenanceService, nil, options)
}

func NewRouterWithPhaseThirteenServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService,
	impactService ImpactService, options RouterOptions,
) http.Handler {
	return newRouterWithServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, recommendationService, generationService, validationService, repairService,
		reviewService, contextService, evaluationService, provenanceService, impactService, nil,
		document.DefaultMaxUploadBytes, nil, nil, nil, nil, nil, nil, nil, nil, options)
}

func NewRouterWithDocumentServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService,
	impactService ImpactService, documentService DocumentService, documentMaxUploadBytes int64,
	options RouterOptions,
) http.Handler {
	return newRouterWithServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, recommendationService, generationService, validationService, repairService,
		reviewService, contextService, evaluationService, provenanceService, impactService,
		documentService, documentMaxUploadBytes, nil, nil, nil, nil, nil, nil, nil, nil, options)
}

func NewRouterWithDocumentWorkflowServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService,
	impactService ImpactService, documentService DocumentService, documentMaxUploadBytes int64,
	documentIndexService DocumentIndexService, requirementService RequirementWorkflowService,
	testCaseService TestCaseWorkflowService, options RouterOptions,
) http.Handler {
	return newRouterWithServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, recommendationService, generationService, validationService, repairService,
		reviewService, contextService, evaluationService, provenanceService, impactService,
		documentService, documentMaxUploadBytes, documentIndexService, requirementService,
		testCaseService, nil, nil, nil, nil, nil, options)
}

func NewRouterWithDocumentDrivenServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService,
	impactService ImpactService, documentService DocumentService, documentMaxUploadBytes int64,
	documentIndexService DocumentIndexService, requirementService RequirementWorkflowService,
	testCaseService TestCaseWorkflowService, reportService ReportService, scopeService ScopeService,
	automationService AutomationService, executionService ExecutionService, options RouterOptions,
) http.Handler {
	return newRouterWithServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, recommendationService, generationService, validationService, repairService,
		reviewService, contextService, evaluationService, provenanceService, impactService,
		documentService, documentMaxUploadBytes, documentIndexService, requirementService,
		testCaseService, reportService, scopeService, automationService, executionService, nil, options)
}

func NewRouterWithUV04Services(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService,
	impactService ImpactService, documentService DocumentService, documentMaxUploadBytes int64,
	documentIndexService DocumentIndexService, requirementService RequirementWorkflowService,
	testCaseService TestCaseWorkflowService, reportService ReportService, scopeService ScopeService,
	automationService AutomationService, executionService ExecutionService,
	documentWorkflowService DocumentWorkflowJobService, options RouterOptions,
) http.Handler {
	return newRouterWithServices(logger, checker, projectService, analysisService, webhookHandler,
		knowledgeService, recommendationService, generationService, validationService, repairService,
		reviewService, contextService, evaluationService, provenanceService, impactService,
		documentService, documentMaxUploadBytes, documentIndexService, requirementService,
		testCaseService, reportService, scopeService, automationService, executionService,
		documentWorkflowService, options)
}

func newRouterWithServices(logger *slog.Logger, checker ReadinessChecker,
	projectService *project.Service, analysisService AnalysisService, webhookHandler http.Handler,
	knowledgeService KnowledgeService, recommendationService RecommendationService,
	generationService GenerationService, validationService ValidationService,
	repairService RepairService, reviewService ReviewService, contextService AnalysisContextService,
	evaluationService EvaluationService, provenanceService ProvenanceService,
	impactService ImpactService, documentService DocumentService, documentMaxUploadBytes int64,
	documentIndexService DocumentIndexService, requirementService RequirementWorkflowService,
	testCaseService TestCaseWorkflowService, reportService ReportService, scopeService ScopeService,
	automationService AutomationService, executionService ExecutionService,
	documentWorkflowService DocumentWorkflowJobService,
	options RouterOptions,
) http.Handler {
	mux := http.NewServeMux()
	health := healthHandler{checker: checker}
	projects := projectHandler{service: projectService}

	mux.HandleFunc("GET /health", health.live)
	mux.HandleFunc("GET /ready", health.ready)
	mux.HandleFunc("POST /api/projects", projects.create)
	mux.HandleFunc("GET /api/projects", projects.list)
	mux.HandleFunc("GET /api/projects/{id}", projects.get)
	mux.HandleFunc("POST /api/projects/{id}/pipeline-mode", projects.setPipelineMode)
	if knowledgeService != nil {
		indexes := knowledgeHandler{service: knowledgeService}
		mux.HandleFunc("POST /api/projects/{id}/index", indexes.requestIndex)
		mux.HandleFunc("GET /api/projects/{id}/index/status", indexes.status)
	}
	if analysisService != nil {
		analyses := analysisHandler{service: analysisService}
		mux.HandleFunc("GET /api/analyses", analyses.list)
		mux.HandleFunc("GET /api/analyses/{id}", analyses.get)
		mux.HandleFunc("GET /api/analyses/{id}/changes", analyses.changes)
	}
	if recommendationService != nil {
		recommendations := recommendationHandler{service: recommendationService}
		mux.HandleFunc("GET /api/analyses/{id}/recommendations", recommendations.list)
	}
	if generationService != nil {
		generatedTests := generationHandler{service: generationService}
		mux.HandleFunc("GET /api/analyses/{id}/generated-tests", generatedTests.list)
	}
	if validationService != nil {
		validations := validationHandler{service: validationService}
		mux.HandleFunc("GET /api/analyses/{id}/validations", validations.list)
	}
	if repairService != nil {
		repairs := repairHandler{service: repairService}
		mux.HandleFunc("GET /api/analyses/{id}/repairs", repairs.list)
	}
	if reviewService != nil {
		reviews := reviewHandler{service: reviewService}
		mux.HandleFunc("GET /api/analyses/{id}/reviews", reviews.list)
		mux.HandleFunc("POST /api/generated-tests/{id}/accept", reviews.accept)
		mux.HandleFunc("POST /api/generated-tests/{id}/reject", reviews.reject)
	}
	if contextService != nil {
		contexts := analysisContextHandler{service: contextService}
		mux.HandleFunc("GET /api/analyses/{id}/context", contexts.list)
	}
	if evaluationService != nil {
		evaluations := evaluationHandler{service: evaluationService}
		mux.HandleFunc("GET /api/evaluations", evaluations.list)
		mux.HandleFunc("GET /api/evaluations/{id}", evaluations.get)
	}
	if provenanceService != nil {
		evidence := provenanceHandler{service: provenanceService}
		mux.HandleFunc("GET /api/analyses/{id}/evidence", evidence.get)
		mux.HandleFunc("GET /api/analyses/{id}/export", evidence.export)
	}
	if impactService != nil {
		impacts := impactHandler{service: impactService}
		mux.HandleFunc("GET /api/analyses/{id}/impact", impacts.get)
	}
	if documentService != nil {
		documents := documentHandler{service: documentService, maxUploadBytes: documentMaxUploadBytes}
		mux.HandleFunc("POST /api/document-sets", documents.createSet)
		mux.HandleFunc("GET /api/document-sets", documents.listSets)
		mux.HandleFunc("GET /api/document-metrics", documents.metrics)
		mux.HandleFunc("GET /api/document-sets/{id}", documents.getSet)
		mux.HandleFunc("POST /api/document-sets/{id}/lifecycle", documents.lifecycle)
		mux.HandleFunc("GET /api/document-sets/{id}/purge", documents.purgePreview)
		mux.HandleFunc("POST /api/document-sets/{id}/purge", documents.purge)
		mux.HandleFunc("GET /api/document-sets/{id}/ai-budget", documents.aiBudget)
		mux.HandleFunc("POST /api/document-sets/{id}/documents", documents.upload)
		mux.HandleFunc("GET /api/document-sets/{id}/documents", documents.listDocuments)
		mux.HandleFunc("POST /api/document-sets/{id}/documents/{documentID}/versions", documents.upload)
		mux.HandleFunc("GET /api/document-sets/{id}/documents/{documentID}/versions", documents.listVersions)
		mux.HandleFunc("GET /api/documents/{id}/versions/{version}", documents.getVersion)
	}
	if documentIndexService != nil {
		indexes := documentIndexHandler{service: documentIndexService}
		mux.HandleFunc("POST /api/document-sets/{id}/index", indexes.index)
		mux.HandleFunc("GET /api/document-sets/{id}/index", indexes.status)
		mux.HandleFunc("GET /api/document-sets/{id}/chunks", indexes.chunks)
		mux.HandleFunc("POST /api/document-sets/{id}/retrieve", indexes.retrieve)
		mux.HandleFunc("POST /api/document-versions/{id}/review", indexes.reviewVersion)
	}
	if requirementService != nil {
		requirements := requirementWorkflowHandler{service: requirementService}
		mux.HandleFunc("POST /api/document-sets/{id}/requirements/extract", requirements.extract)
		mux.HandleFunc("GET /api/document-sets/{id}/requirements/extraction", requirements.extractionStatus)
		mux.HandleFunc("GET /api/document-sets/{id}/requirements", requirements.list)
		mux.HandleFunc("GET /api/document-sets/{id}/requirement-conflicts", requirements.conflicts)
		mux.HandleFunc("GET /api/document-sets/{id}/open-questions", requirements.questions)
		mux.HandleFunc("GET /api/requirements/{id}", requirements.get)
		mux.HandleFunc("POST /api/requirements/{id}/review", requirements.review)
		mux.HandleFunc("POST /api/document-sets/{id}/requirement-review/{action}", requirements.reviewScope)
		mux.HandleFunc("GET /api/document-sets/{id}/requirement-review/{action}", requirements.reviewScope)
	}
	if testCaseService != nil {
		testCases := testCaseWorkflowHandler{service: testCaseService}
		mux.HandleFunc("POST /api/document-sets/{id}/test-cases/generate", testCases.generate)
		mux.HandleFunc("POST /api/document-sets/{id}/test-cases/regenerate", testCases.regenerate)
		mux.HandleFunc("GET /api/document-sets/{id}/test-cases", testCases.list)
		mux.HandleFunc("GET /api/document-sets/{id}/test-case-families", testCases.listFamilies)
		mux.HandleFunc("GET /api/document-sets/{id}/coverage", testCases.coverage)
		mux.HandleFunc("GET /api/test-cases/{id}", testCases.get)
		mux.HandleFunc("POST /api/test-cases/{id}/review", testCases.review)
		mux.HandleFunc("POST /api/test-cases/bulk-review", testCases.bulkReview)
		mux.HandleFunc("GET /api/test-case-families/{id}", testCases.getFamily)
		mux.HandleFunc("GET /api/test-case-families/{id}/versions", testCases.listVersions)
		mux.HandleFunc("POST /api/test-case-families/{id}/versions", testCases.createRevision)
		mux.HandleFunc("GET /api/test-case-families/{id}/diff", testCases.diff)
		mux.HandleFunc("GET /api/test-case-families/{id}/{action}", testCases.workspace)
		mux.HandleFunc("GET /api/test-cases/{id}/{action}", testCases.workspace)
		mux.HandleFunc("POST /api/document-sets/{id}/suite-releases/preview", testCases.previewRelease)
		mux.HandleFunc("POST /api/test-case-families/{id}/restore", testCases.restore)
		mux.HandleFunc("POST /api/test-case-families/{id}/archive", testCases.archive)
		mux.HandleFunc("POST /api/document-sets/{id}/suite-releases", testCases.publishRelease)
		mux.HandleFunc("GET /api/document-sets/{id}/suite-releases", testCases.listReleases)
		mux.HandleFunc("GET /api/test-suite-releases/{id}", testCases.getRelease)
	}
	if reportService != nil {
		reports := reportHandler{service: reportService}
		mux.HandleFunc("POST /api/document-sets/{id}/exports", reports.export)
		mux.HandleFunc("GET /api/document-sets/{id}/exports", reports.list)
		mux.HandleFunc("GET /api/test-exports/{id}/download", reports.download)
	}
	if scopeService != nil {
		scopes := scopeHandler{service: scopeService}
		mux.HandleFunc("GET /api/projects/{id}/document-baseline", scopes.baseline)
		mux.HandleFunc("POST /api/projects/{id}/document-baseline", scopes.selectBaseline)
		mux.HandleFunc("GET /api/analyses/{id}/test-scope", scopes.get)
		mux.HandleFunc("POST /api/analyses/{id}/test-scope", scopes.decide)
	}
	if automationService != nil {
		automations := automationHandler{service: automationService}
		mux.HandleFunc("POST /api/analyses/{id}/automation/generate", automations.generate)
		mux.HandleFunc("GET /api/test-cases/{id}/automation", automations.history)
		mux.HandleFunc("POST /api/automation-artifacts/{id}/review", automations.review)
		if repairService, ok := automationService.(AutomationRepairService); ok {
			repairs := automationRepairHandler{service: repairService}
			mux.HandleFunc("POST /api/test-run-items/{id}/repair", repairs.request)
			mux.HandleFunc("GET /api/test-run-items/{id}/repairs", repairs.list)
		}
	}
	if executionService != nil {
		executions := executionHandler{service: executionService}
		mux.HandleFunc("GET /api/test-runs/{id}", executions.get)
		mux.HandleFunc("POST /api/test-runs/{id}/execute", executions.request)
		mux.HandleFunc("POST /api/test-run-items/{id}/classification", executions.classify)
	}
	if documentWorkflowService != nil {
		workflows := documentWorkflowJobHandler{service: documentWorkflowService}
		mux.HandleFunc("GET /api/document-sets/{id}/workflow", workflows.read)
		mux.HandleFunc("POST /api/document-sets/{id}/source-review", workflows.sourceCommand)
		mux.HandleFunc("POST /api/document-sets/{id}/workflow-operations", workflows.enqueue)
		mux.HandleFunc("GET /api/document-workflow-jobs/{id}", workflows.get)
		mux.HandleFunc("POST /api/document-workflow-jobs/{id}/retry", workflows.retry)
		mux.HandleFunc("POST /api/document-workflow-jobs/{id}/cancel", workflows.cancel)
	}
	if webhookHandler != nil {
		mux.Handle("POST /api/webhooks/gitlab", webhookHandler)
		mux.Handle("POST /api/webhooks/github", webhookHandler)
	}

	limiter := newClientRateLimiter(options)
	return requestLogger(logger, securityHeaders(limiter.middleware(
		authorizationMiddleware(options, legacyDeprecationHeaders(mux)))))
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := requestID(r.Header.Get("X-Request-ID"))
		w.Header().Set("X-Request-ID", requestID)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info("http request", "request_id", requestID, "method", r.Method, "path", r.URL.Path,
			"status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func requestID(provided string) string {
	provided = strings.TrimSpace(provided)
	if provided != "" && len(provided) <= 128 {
		return provided
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err == nil {
		return hex.EncodeToString(random)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
