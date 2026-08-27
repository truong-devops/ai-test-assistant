package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	mux := http.NewServeMux()
	health := healthHandler{checker: checker}
	projects := projectHandler{service: projectService}

	mux.HandleFunc("GET /health", health.live)
	mux.HandleFunc("GET /ready", health.ready)
	mux.HandleFunc("POST /api/projects", projects.create)
	mux.HandleFunc("GET /api/projects", projects.list)
	mux.HandleFunc("GET /api/projects/{id}", projects.get)
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
	if webhookHandler != nil {
		mux.Handle("POST /api/webhooks/gitlab", webhookHandler)
	}

	return requestLogger(logger, mux)
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
