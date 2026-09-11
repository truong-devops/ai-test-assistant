package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type DocumentIndexService interface {
	Index(context.Context, int64) (document.IndexStatus, error)
	Status(context.Context, int64) (document.IndexStatus, error)
	ListChunks(context.Context, int64, string, string, int) ([]document.SemanticChunk, error)
	Retrieve(context.Context, document.RetrievalQuery) ([]document.SemanticChunk, error)
	ReviewVersion(context.Context, int64, document.VersionReviewInput) (document.VersionReview, error)
}

type RequirementWorkflowService interface {
	Extract(context.Context, int64) (requirement.ExtractionSummary, error)
	List(context.Context, requirement.Filter) ([]requirement.Requirement, error)
	Get(context.Context, int64) (requirement.Detail, error)
	Review(context.Context, int64, requirement.ReviewInput) (requirement.Detail, error)
	ListConflicts(context.Context, int64) ([]requirement.Conflict, error)
	ListOpenQuestions(context.Context, int64) ([]requirement.OpenQuestion, error)
}

type TestCaseWorkflowService interface {
	Generate(context.Context, int64) (testcase.GenerateSummary, error)
	Regenerate(context.Context, int64) (testcase.GenerateSummary, error)
	List(context.Context, int64) ([]testcase.TestCase, error)
	Get(context.Context, int64) (testcase.Detail, error)
	Review(context.Context, int64, testcase.ReviewInput) (testcase.Detail, error)
	BulkReview(context.Context, testcase.BulkReviewInput) ([]testcase.Detail, error)
	Coverage(context.Context, int64) (testcase.CoverageReport, error)
}

type documentIndexHandler struct{ service DocumentIndexService }

func (h documentIndexHandler) index(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.Index(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not index document set")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"index": result})
}

func (h documentIndexHandler) status(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.Status(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not get document index status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"index": result})
}

func (h documentIndexHandler) chunks(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	results, err := h.service.ListChunks(r.Context(), setID, r.URL.Query().Get("type"),
		r.URL.Query().Get("flow_type"), limit)
	if err != nil {
		writeWorkflowError(w, err, "could not list document chunks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chunks": results})
}

func (h documentIndexHandler) retrieve(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	var input document.RetrievalQuery
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	input.DocumentSetID = setID
	results, err := h.service.Retrieve(r.Context(), input)
	if err != nil {
		writeWorkflowError(w, err, "could not retrieve document evidence")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (h documentIndexHandler) reviewVersion(w http.ResponseWriter, r *http.Request) {
	versionID, ok := positiveInt64Path(w, r, "id", "document version")
	if !ok {
		return
	}
	var input document.VersionReviewInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.ReviewVersion(r.Context(), versionID, input)
	if err != nil {
		writeWorkflowError(w, err, "could not review document version")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type requirementWorkflowHandler struct{ service RequirementWorkflowService }

func (h requirementWorkflowHandler) extract(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.Extract(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not extract requirements")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h requirementWorkflowHandler) list(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	documentID, _ := strconv.ParseInt(r.URL.Query().Get("document_id"), 10, 64)
	results, err := h.service.List(r.Context(), requirement.Filter{DocumentSetID: setID,
		DocumentID: documentID, RequirementType: r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"), Risk: r.URL.Query().Get("risk"),
		Actor: r.URL.Query().Get("actor"), FlowType: r.URL.Query().Get("flow_type")})
	if err != nil {
		writeWorkflowError(w, err, "could not list requirements")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requirements": results})
}

func (h requirementWorkflowHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "requirement")
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeWorkflowError(w, err, "could not get requirement")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h requirementWorkflowHandler) review(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "requirement")
	if !ok {
		return
	}
	var input requirement.ReviewInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Review(r.Context(), id, input)
	if err != nil {
		writeWorkflowError(w, err, "could not review requirement")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h requirementWorkflowHandler) conflicts(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	results, err := h.service.ListConflicts(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not list requirement conflicts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": results})
}

func (h requirementWorkflowHandler) questions(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	results, err := h.service.ListOpenQuestions(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not list open questions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"open_questions": results})
}

type testCaseWorkflowHandler struct{ service TestCaseWorkflowService }

func (h testCaseWorkflowHandler) generate(w http.ResponseWriter, r *http.Request) {
	h.runGenerate(w, r, false)
}

func (h testCaseWorkflowHandler) regenerate(w http.ResponseWriter, r *http.Request) {
	h.runGenerate(w, r, true)
}

func (h testCaseWorkflowHandler) runGenerate(w http.ResponseWriter, r *http.Request, regenerate bool) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	var result testcase.GenerateSummary
	var err error
	if regenerate {
		result, err = h.service.Regenerate(r.Context(), setID)
	} else {
		result, err = h.service.Generate(r.Context(), setID)
	}
	if err != nil {
		writeWorkflowError(w, err, "could not generate test cases")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) list(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	results, err := h.service.List(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not list test cases")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"test_cases": results})
}

func (h testCaseWorkflowHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "test case")
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeWorkflowError(w, err, "could not get test case")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) review(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "test case")
	if !ok {
		return
	}
	var input testcase.ReviewInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Review(r.Context(), id, input)
	if err != nil {
		writeWorkflowError(w, err, "could not review test case")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) bulkReview(w http.ResponseWriter, r *http.Request) {
	var input testcase.BulkReviewInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	results, err := h.service.BulkReview(r.Context(), input)
	if err != nil {
		writeWorkflowError(w, err, "could not bulk-review test cases")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"test_cases": results})
}

func (h testCaseWorkflowHandler) coverage(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.Coverage(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not calculate coverage")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeWorkflowJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}

func writeWorkflowError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, document.ErrNotFound), errors.Is(err, requirement.ErrNotFound),
		errors.Is(err, testcase.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, document.ErrInvalidInput), errors.Is(err, document.ErrInvalidSearch),
		errors.Is(err, requirement.ErrInvalidInput), errors.Is(err, testcase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, document.ErrNoParsedDocuments), errors.Is(err, document.ErrIndexNotReady),
		errors.Is(err, document.ErrSourceReviewBlocked),
		errors.Is(err, requirement.ErrNoIndex), errors.Is(err, requirement.ErrMissingEvidence),
		errors.Is(err, requirement.ErrReviewBlocked), errors.Is(err, testcase.ErrNoApprovedSource),
		errors.Is(err, testcase.ErrReviewBlocked), errors.Is(err, document.ErrUnapprovedEvidence):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}
