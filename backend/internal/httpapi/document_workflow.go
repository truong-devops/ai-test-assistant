package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type DocumentIndexService interface {
	IndexWithOptions(context.Context, int64, document.IndexOptions) (document.IndexStatus, error)
	Status(context.Context, int64) (document.IndexStatus, error)
	ListGenerationChunks(context.Context, int64, int64, string, string, int) ([]document.SemanticChunk, error)
	Retrieve(context.Context, document.RetrievalQuery) ([]document.SemanticChunk, error)
	ReviewVersion(context.Context, int64, document.VersionReviewInput) (document.VersionReview, error)
}

type RequirementWorkflowService interface {
	RequestExtraction(context.Context, int64, string) (requirement.ExtractionJob, error)
	ExtractionStatus(context.Context, int64) (requirement.ExtractionJob, error)
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
	ListFamilies(context.Context, int64) ([]testcase.Family, error)
	GetFamily(context.Context, int64) (testcase.Family, error)
	ListVersions(context.Context, int64) ([]testcase.TestCase, error)
	CreateRevision(context.Context, int64, testcase.CreateRevisionInput, string, string) (testcase.RevisionResult, error)
	Restore(context.Context, int64, testcase.RestoreInput, string, string) (testcase.RevisionResult, error)
	Diff(context.Context, int64, int64, int64) (testcase.RevisionDiff, error)
	Archive(context.Context, int64, testcase.ArchiveInput, string) (testcase.Family, error)
	PublishRelease(context.Context, int64, testcase.PublishReleaseInput, string, string) (testcase.SuiteRelease, bool, error)
	ListReleases(context.Context, int64) ([]testcase.SuiteRelease, error)
	GetRelease(context.Context, int64) (testcase.SuiteRelease, error)
}

type documentIndexHandler struct{ service DocumentIndexService }

func (h documentIndexHandler) index(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	var input document.IndexOptions
	if r.ContentLength != 0 && !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.IndexWithOptions(r.Context(), setID, input)
	if err != nil {
		var blocked *document.SourceSnapshotBlockedError
		if errors.As(err, &blocked) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": blocked.Error(),
				"code": "SOURCE_SNAPSHOT_BLOCKED", "issues": blocked.Issues})
			return
		}
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
	generation, _ := strconv.ParseInt(r.URL.Query().Get("generation"), 10, 64)
	results, err := h.service.ListGenerationChunks(r.Context(), setID, generation,
		r.URL.Query().Get("type"), r.URL.Query().Get("flow_type"), limit)
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
	var input struct {
		RequestedBy string `json:"requested_by"`
	}
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.RequestExtraction(r.Context(), setID, input.RequestedBy)
	if err != nil {
		writeWorkflowError(w, err, "could not extract requirements")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": result,
		"status_url": "/api/document-sets/" + strconv.FormatInt(setID, 10) + "/requirements/extraction"})
}

func (h requirementWorkflowHandler) extractionStatus(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	result, err := h.service.ExtractionStatus(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not get requirement extraction status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": result})
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
	input.Comment += "\nDisplay name: " + input.ReviewerName
	input.ReviewerName = workflowActor(r)
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		input.CommandKey = "single:" + key
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
	if result.TestCase.ID != id {
		w.Header().Set("Location", "/api/test-cases/"+strconv.FormatInt(result.TestCase.ID, 10))
		w.Header().Set("Deprecation", "true")
		w.Header().Set("Link", `</api/test-case-families/`+
			strconv.FormatInt(result.TestCase.FamilyID, 10)+`/versions>; rel="successor-version"`)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) listFamilies(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	results, err := h.service.ListFamilies(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not list test case families")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"families": results})
}

func (h testCaseWorkflowHandler) getFamily(w http.ResponseWriter, r *http.Request) {
	familyID, ok := positiveInt64Path(w, r, "id", "test case family")
	if !ok {
		return
	}
	result, err := h.service.GetFamily(r.Context(), familyID)
	if err != nil {
		writeWorkflowError(w, err, "could not get test case family")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) listVersions(w http.ResponseWriter, r *http.Request) {
	familyID, ok := positiveInt64Path(w, r, "id", "test case family")
	if !ok {
		return
	}
	results, err := h.service.ListVersions(r.Context(), familyID)
	if err != nil {
		writeWorkflowError(w, err, "could not list test case revisions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": results})
}

func (h testCaseWorkflowHandler) createRevision(w http.ResponseWriter, r *http.Request) {
	familyID, ok := positiveInt64Path(w, r, "id", "test case family")
	if !ok {
		return
	}
	var input testcase.CreateRevisionInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.CreateRevision(r.Context(), familyID, input,
		r.Header.Get("Idempotency-Key"), workflowActor(r))
	if err != nil {
		writeWorkflowError(w, err, "could not create test case revision")
		return
	}
	w.Header().Set("Location", result.Location)
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

func (h testCaseWorkflowHandler) restore(w http.ResponseWriter, r *http.Request) {
	familyID, ok := positiveInt64Path(w, r, "id", "test case family")
	if !ok {
		return
	}
	var input testcase.RestoreInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Restore(r.Context(), familyID, input,
		r.Header.Get("Idempotency-Key"), workflowActor(r))
	if err != nil {
		writeWorkflowError(w, err, "could not restore test case revision")
		return
	}
	w.Header().Set("Location", result.Location)
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

func (h testCaseWorkflowHandler) diff(w http.ResponseWriter, r *http.Request) {
	familyID, ok := positiveInt64Path(w, r, "id", "test case family")
	if !ok {
		return
	}
	fromID, errFrom := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	toID, errTo := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if errFrom != nil || errTo != nil || fromID <= 0 || toID <= 0 {
		writeError(w, http.StatusBadRequest, "from and to revision IDs are required")
		return
	}
	result, err := h.service.Diff(r.Context(), familyID, fromID, toID)
	if err != nil {
		writeWorkflowError(w, err, "could not diff test case revisions")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) archive(w http.ResponseWriter, r *http.Request) {
	familyID, ok := positiveInt64Path(w, r, "id", "test case family")
	if !ok {
		return
	}
	var input testcase.ArchiveInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := h.service.Archive(r.Context(), familyID, input, workflowActor(r))
	if err != nil {
		writeWorkflowError(w, err, "could not archive test case family")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h testCaseWorkflowHandler) publishRelease(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	var input testcase.PublishReleaseInput
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, created, err := h.service.PublishRelease(r.Context(), setID, input,
		r.Header.Get("Idempotency-Key"), workflowActor(r))
	if err != nil {
		writeWorkflowError(w, err, "could not publish test suite release")
		return
	}
	w.Header().Set("Location", "/api/test-suite-releases/"+strconv.FormatInt(result.ID, 10))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

func (h testCaseWorkflowHandler) listReleases(w http.ResponseWriter, r *http.Request) {
	setID, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	results, err := h.service.ListReleases(r.Context(), setID)
	if err != nil {
		writeWorkflowError(w, err, "could not list test suite releases")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": results})
}

func (h testCaseWorkflowHandler) getRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, ok := positiveInt64Path(w, r, "id", "test suite release")
	if !ok {
		return
	}
	result, err := h.service.GetRelease(r.Context(), releaseID)
	if err != nil {
		writeWorkflowError(w, err, "could not get test suite release")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func workflowActor(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Authenticated-Actor")); value != "" {
		return value
	}
	return "API_USER"
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
	var revisionConflict *testcase.RevisionConflictError
	switch {
	case errors.Is(err, requirement.ErrRevisionConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "REQUIREMENT_REVISION_CONFLICT"})
	case errors.Is(err, requirement.ErrIdempotencyConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "IDEMPOTENCY_KEY_REUSED"})
	case errors.As(err, &revisionConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": revisionConflict.Error(),
			"code":                "TESTCASE_HEAD_CONFLICT",
			"current_revision_id": revisionConflict.CurrentRevisionID,
			"current_head_token":  revisionConflict.CurrentHeadToken})
	case errors.Is(err, testcase.ErrIdempotencyConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(),
			"code": "IDEMPOTENCY_KEY_REUSED"})
	case errors.Is(err, testcase.ErrEvidenceInvalid):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(),
			"code": "TESTCASE_EVIDENCE_INVALID"})
	case errors.Is(err, document.ErrNotFound), errors.Is(err, requirement.ErrNotFound),
		errors.Is(err, testcase.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, document.ErrInvalidInput), errors.Is(err, document.ErrInvalidSearch),
		errors.Is(err, requirement.ErrInvalidInput), errors.Is(err, testcase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, document.ErrNoParsedDocuments), errors.Is(err, document.ErrIndexNotReady),
		errors.Is(err, document.ErrSourceReviewBlocked), errors.Is(err, document.ErrSourceSnapshotBlocked),
		errors.Is(err, document.ErrIndexStale), errors.Is(err, requirement.ErrStaleIndex),
		errors.Is(err, requirement.ErrNoIndex), errors.Is(err, requirement.ErrMissingEvidence),
		errors.Is(err, requirement.ErrReviewBlocked), errors.Is(err, requirement.ErrSourceNotApproved),
		errors.Is(err, testcase.ErrNoApprovedSource),
		errors.Is(err, testcase.ErrReviewBlocked), errors.Is(err, document.ErrUnapprovedEvidence):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, testcase.ErrFamilyArchived):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(),
			"code": "TESTCASE_FAMILY_ARCHIVED"})
	case errors.Is(err, testcase.ErrReleaseScope):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(),
			"code": "SUITE_RELEASE_SCOPE_INVALID"})
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}
