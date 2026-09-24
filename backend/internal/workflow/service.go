package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type IndexService interface {
	Status(context.Context, int64) (document.IndexStatus, error)
	IndexWithOptions(context.Context, int64, document.IndexOptions) (document.IndexStatus, error)
}

type ExtractionService interface {
	RequestExtraction(context.Context, int64, string) (requirement.ExtractionJob, error)
}

type TestCaseService interface {
	GeneratePinned(context.Context, int64, testcase.GenerationBaseline) (testcase.GenerateSummary, error)
}

type Service struct {
	repository  *Repository
	index       IndexService
	extraction  ExtractionService
	testcases   TestCaseService
	maxAttempts int
}

func NewService(repository *Repository, index IndexService, extraction ExtractionService,
	testcases TestCaseService, maxAttempts int,
) *Service {
	if maxAttempts < 1 || maxAttempts > 20 {
		maxAttempts = 3
	}
	return &Service{repository: repository, index: index, extraction: extraction,
		testcases: testcases, maxAttempts: maxAttempts}
}

func (s *Service) Enqueue(ctx context.Context, setID int64, input OperationInput,
	idempotencyKey, role string,
) (Job, bool, error) {
	input.Operation = strings.ToUpper(strings.TrimSpace(input.Operation))
	input.GenerationScope = strings.ToUpper(strings.TrimSpace(input.GenerationScope))
	input.RequestedBy = strings.TrimSpace(input.RequestedBy)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if input.RequestedBy == "" {
		input.RequestedBy = "API_USER"
	}
	if setID <= 0 || !validOperation(input.Operation) || idempotencyKey == "" {
		return Job{}, false, ErrInvalidInput
	}
	if input.GenerationScope != "" && (!input.ReviewProposals || !slices.Contains([]string{"ALL", "SELECTED", "AFFECTED"}, input.GenerationScope)) {
		return Job{}, false, ErrInvalidInput
	}
	if input.Operation != OperationGenerate && (len(input.RequirementIDs) > 0 || input.ReviewProposals || input.GenerationScope != "") {
		return Job{}, false, ErrInvalidInput
	}
	selection, err := generationSelection(input.RequirementIDs)
	if err != nil {
		return Job{}, false, err
	}
	input.RequirementIDs = selection
	if !roleAtLeast(role, "editor") {
		return Job{}, false, ErrForbidden
	}
	if existing, found, err := s.repository.ExistingByKey(ctx, setID, idempotencyKey); err != nil {
		return Job{}, false, err
	} else if found {
		if !sameIdempotentCommand(existing, input) {
			return Job{}, false, ErrIdempotencyConflict
		}
		return existing, false, nil
	}
	facts, err := s.repository.readFacts(ctx, setID)
	if err != nil {
		return Job{}, false, err
	}
	if input.Operation != OperationIndex && !input.ReviewProposals && !budgetAvailable(facts) {
		reason := BlockingReason{Code: "AI_BUDGET_EXHAUSTED",
			Message: "AI budget has no remaining capacity", Step: "REQUIREMENTS",
			NextAction: "ADJUST_AI_BUDGET"}
		return Job{}, false, &BlockedError{Code: reason.Code, Message: reason.Message,
			BlockedBy: []BlockingReason{reason}, NextAction: reason.NextAction}
	}
	var snapshot json.RawMessage
	var inputHash string
	err = nil
	switch input.Operation {
	case OperationIndex:
		snapshot, inputHash, err = s.repository.BuildIndexSnapshot(ctx, setID,
			input.ExcludedVersionIDs)
	case OperationGenerate:
		if input.ReviewProposals {
			snapshot, inputHash, err = s.repository.buildProposalSnapshot(ctx, setID, input)
		} else {
			snapshot, inputHash, err = s.repository.BuildGenerateSnapshotForRequirements(ctx, setID, input.RequirementIDs)
		}
	case OperationExtract:
		if s.index == nil || s.extraction == nil {
			return Job{}, false, ErrInvalidInput
		}
		var status document.IndexStatus
		status, err = s.index.Status(ctx, setID)
		if err == nil {
			switch {
			case status.Status != document.IndexReady:
				err = requirement.ErrNoIndex
			case status.Freshness != document.IndexFreshnessCurrent:
				err = requirement.ErrStaleIndex
			case !status.ExtractionReady:
				err = requirement.ErrSourceNotApproved
			default:
				snapshot, inputHash, err = hashJSON(map[string]any{
					"index_generation":   status.Generation,
					"source_snapshot_id": status.SourceSnapshotID,
					"source_revision":    status.IndexedSourceRevision,
					"total_chunks":       status.ChunkCount,
				})
			}
		}
	}
	if err != nil {
		return Job{}, false, err
	}
	if input.ReviewProposals && !budgetAvailable(facts) {
		var pin generateInputSnapshot
		_ = json.Unmarshal(snapshot, &pin)
		if len(pin.Requirements) > 0 {
			return Job{}, false, &BlockedError{Code: "AI_BUDGET_EXHAUSTED", Message: "AI budget has no remaining capacity"}
		}
	}
	if input.Operation == OperationExtract {
		external, requestErr := s.extraction.RequestExtraction(ctx, setID, input.RequestedBy)
		if requestErr != nil {
			return Job{}, false, requestErr
		}
		return s.repository.EnqueueDelegated(ctx, setID, input.Operation, input.RequestedBy,
			idempotencyKey, snapshot, inputHash, s.maxAttempts,
			"REQUIREMENT_EXTRACTION", external.ID)
	}
	return s.repository.EnqueueNative(ctx, setID, input.Operation, input.RequestedBy,
		idempotencyKey, snapshot, inputHash, s.maxAttempts)
}

func sameIdempotentCommand(existing Job, input OperationInput) bool {
	if existing.Operation != input.Operation {
		return false
	}
	if input.Operation == OperationGenerate {
		var snapshot generateInputSnapshot
		if json.Unmarshal(existing.InputSnapshot, &snapshot) != nil {
			return false
		}
		if snapshot.ReviewProposals != input.ReviewProposals {
			return false
		}
		if snapshot.RequestedScope != input.GenerationScope {
			return false
		}
		wanted, err := generationSelection(input.RequirementIDs)
		if err != nil {
			return false
		}
		stored, err := generationSelection(snapshot.SelectedRequirementIDs)
		return err == nil && slices.Equal(wanted, stored)
	}
	if input.Operation != OperationIndex {
		return true
	}
	var snapshot indexInputSnapshot
	if json.Unmarshal(existing.InputSnapshot, &snapshot) != nil {
		return false
	}
	wanted := append([]int64(nil), input.ExcludedVersionIDs...)
	stored := append([]int64(nil), snapshot.ExcludedVersionIDs...)
	slices.Sort(wanted)
	slices.Sort(stored)
	wanted = slices.Compact(wanted)
	stored = slices.Compact(stored)
	return slices.Equal(wanted, stored)
}

func generationSelection(ids []int64) ([]int64, error) {
	if len(ids) > 100 {
		return nil, ErrInvalidInput
	}
	selected := append([]int64{}, ids...)
	slices.Sort(selected)
	for index, id := range selected {
		if id <= 0 || index > 0 && selected[index-1] == id {
			return nil, ErrInvalidInput
		}
	}
	return selected, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Job, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) Retry(ctx context.Context, id int64, expectedRevision int,
	role string,
) (Job, error) {
	if !roleAtLeast(role, "editor") {
		return Job{}, ErrForbidden
	}
	return s.repository.Retry(ctx, id, expectedRevision)
}

func (s *Service) Cancel(ctx context.Context, id int64, expectedRevision int,
	role string,
) (Job, error) {
	if !roleAtLeast(role, "editor") {
		return Job{}, ErrForbidden
	}
	return s.repository.Cancel(ctx, id, expectedRevision)
}

func (s *Service) Process(ctx context.Context, job Job) (any, error) {
	if len(job.Units) != 1 {
		return nil, ErrInvalidInput
	}
	if err := s.repository.ValidateInputSnapshot(ctx, job); err != nil {
		return nil, err
	}
	switch job.Operation {
	case OperationIndex:
		if s.index == nil {
			return nil, ErrInvalidInput
		}
		var snapshot indexInputSnapshot
		if err := json.Unmarshal(job.InputSnapshot, &snapshot); err != nil {
			return nil, ErrInvalidInput
		}
		result, err := s.index.IndexWithOptions(ctx, job.DocumentSetID,
			document.IndexOptions{ExcludedVersionIDs: snapshot.ExcludedVersionIDs})
		if err != nil {
			return nil, err
		}
		return map[string]any{"index_generation": result.Generation,
			"source_snapshot_id": result.SourceSnapshotID, "chunk_count": result.ChunkCount}, nil
	case OperationGenerate:
		if s.testcases == nil {
			return nil, ErrInvalidInput
		}
		var snapshot generateInputSnapshot
		if err := json.Unmarshal(job.InputSnapshot, &snapshot); err != nil {
			return nil, ErrInvalidInput
		}
		if snapshot.PerRequirementUnits {
			return s.processGenerationUnits(ctx, job, snapshot)
		}
		if len(snapshot.Requirements) == 0 {
			return nil, ErrInvalidInput
		}
		ids := make([]int64, 0, len(snapshot.Requirements))
		for _, item := range snapshot.Requirements {
			ids = append(ids, item.ID)
		}
		result, err := s.testcases.GeneratePinned(ctx, job.DocumentSetID, testcase.GenerationBaseline{
			RequirementIDs: ids, WorkflowJobID: job.ID, InputHash: job.InputHash,
			SourceRevision: snapshot.SourceRevision, ReviewProposals: snapshot.ReviewProposals,
			Targets: snapshot.Targets, WorkflowAttempt: job.AttemptCount,
			IncludeRetire: len(snapshot.SelectedRequirementIDs) == 0})
		if err != nil {
			return nil, err
		}
		return map[string]any{"test_suite_id": result.SuiteID,
			"requirement_count": result.RequirementCount, "created_count": result.CreatedCount,
			"reused_count": result.ReusedCount, "suppressed_count": result.SuppressedCount,
			"proposal_count": result.ProposalCount, "review_proposals": snapshot.ReviewProposals}, nil
	default:
		return nil, ErrInvalidInput
	}
}

func (s *Service) Read(ctx context.Context, setID int64, role string) (ReadModel, error) {
	facts, err := s.repository.readFacts(ctx, setID)
	if err != nil {
		return ReadModel{}, err
	}
	jobs, err := s.repository.List(ctx, setID, 20)
	if err != nil {
		return ReadModel{}, err
	}
	result := ReadModel{DocumentSetID: setID, SourceRevision: facts.SourceRevision,
		Steps: []Step{}, BlockingReasons: []BlockingReason{}, ActiveJobs: []Job{},
		RecentJobs: jobs}
	result.GenerationScope, err = s.repository.GenerationScope(ctx, setID)
	if err != nil {
		return ReadModel{}, err
	}
	result.SourceIntents, err = s.repository.SourceIntents(ctx, setID)
	if err != nil {
		return ReadModel{}, err
	}
	for _, item := range jobs {
		if !Terminal(item.Status) {
			result.ActiveJobs = append(result.ActiveJobs, item)
		}
	}
	editable := roleAtLeast(role, "editor") && facts.SetStatus == "ACTIVE"
	reviewer := roleAtLeast(role, "reviewer") && facts.SetStatus == "ACTIVE"
	budgetOK := budgetAvailable(facts)
	result.Capabilities = Capabilities{
		CanUpload: editable, CanManage: roleAtLeast(role, "reviewer"),
		CanIndex:    editable && facts.DocumentCount > 0,
		CanExtract:  editable && facts.ExtractionReady && budgetOK,
		CanGenerate: editable && ((facts.ApprovedRequirements > 0 && budgetOK) || len(result.GenerationScope.RemovedFamilyIDs) > 0),
		CanReview:   reviewer, CanPublish: reviewer && facts.ApprovedTestCases > 0,
		CanRetryJob: editable, CanCancelJob: editable,
		CanAdjustBudget: roleAtLeast(role, "admin"),
	}
	if facts.SetStatus != "ACTIVE" {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "DOCUMENT_SET_INACTIVE", Message: "document set is not active",
			Step: "SOURCE", NextAction: "REACTIVATE_DOCUMENT_SET"})
	}
	if facts.DocumentCount == 0 {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "NO_DOCUMENTS", Message: "upload at least one source document",
			Step: "SOURCE", NextAction: "UPLOAD_DOCUMENT"})
	} else if facts.ParsedCount < facts.DocumentCount && !facts.ExtractionReady {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "SOURCE_PARSE_PENDING", Message: "one or more latest document versions are not parsed",
			Step: "SOURCE", NextAction: "WAIT_FOR_PARSE"})
	}
	indexCurrent := facts.IndexStatus == document.IndexReady &&
		facts.IndexedRevision == facts.SourceRevision
	if !indexCurrent {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "INDEX_NOT_CURRENT", Message: "semantic index is missing or stale",
			Step: "INDEX", NextAction: "INDEX_DOCUMENTS"})
	}
	if indexCurrent && !facts.ExtractionReady {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "SOURCE_NOT_APPROVED", Message: "all included source versions must be approved",
			Step: "REQUIREMENTS", NextAction: "REVIEW_SOURCE"})
	}
	if facts.ApprovedRequirements == 0 && len(result.GenerationScope.RemovedFamilyIDs) == 0 {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "NO_APPROVED_REQUIREMENTS", Message: "no current approved requirements",
			Step: "TESTCASES", NextAction: "REVIEW_REQUIREMENTS"})
	}
	if facts.BudgetRemainingTokens == 0 ||
		facts.CostBudgetConfigured && facts.BudgetRemainingCost == 0 {
		result.BlockingReasons = append(result.BlockingReasons, BlockingReason{
			Code: "AI_BUDGET_EXHAUSTED", Message: "AI budget has no remaining capacity",
			Step: "REQUIREMENTS", NextAction: "ADJUST_AI_BUDGET"})
	}
	activeByOperation := map[string]Job{}
	latestByOperation := map[string]Job{}
	for _, item := range jobs {
		if _, exists := latestByOperation[item.Operation]; !exists {
			latestByOperation[item.Operation] = item
		}
	}
	for _, item := range result.ActiveJobs {
		activeByOperation[item.Operation] = item
	}
	result.Steps = append(result.Steps,
		workflowStep("SOURCE", facts.ParsedCount, facts.DocumentCount,
			facts.ExtractionReady,
			stepReasons(result.BlockingReasons, "SOURCE"), Job{}),
		workflowStep("INDEX", boolInt(indexCurrent), 1, indexCurrent,
			stepReasons(result.BlockingReasons, "INDEX"),
			stepJob(activeByOperation, latestByOperation, OperationIndex)),
		workflowStep("REQUIREMENTS", facts.ApprovedRequirements, max(facts.RequirementCount, 1),
			facts.ApprovedRequirements > 0, stepReasons(result.BlockingReasons, "REQUIREMENTS"),
			stepJob(activeByOperation, latestByOperation, OperationExtract)),
		workflowStep("TESTCASES", facts.ApprovedTestCases, max(facts.TestCaseCount, 1),
			facts.TestCaseCount > 0, stepReasons(result.BlockingReasons, "TESTCASES"),
			stepJob(activeByOperation, latestByOperation, OperationGenerate)),
		workflowStep("RELEASE", boolInt(facts.ReleaseCount > 0), 1, facts.ReleaseCount > 0,
			[]BlockingReason{}, Job{}),
	)
	switch {
	case facts.DocumentCount == 0:
		result.NextAction = "UPLOAD_DOCUMENT"
	case len(result.ActiveJobs) > 0:
		result.NextAction = "WAIT_FOR_ACTIVE_JOB"
	case facts.ParsedCount < facts.DocumentCount && !indexCurrent:
		result.NextAction = "WAIT_FOR_PARSE"
	case result.Capabilities.CanIndex && !indexCurrent:
		result.NextAction = OperationIndex
	case !facts.ExtractionReady:
		result.NextAction = "REVIEW_SOURCE"
	case result.Capabilities.CanGenerate && (len(result.GenerationScope.AffectedRequirementIDs) > 0 || len(result.GenerationScope.RemovedFamilyIDs) > 0):
		result.NextAction = OperationGenerate
	case result.Capabilities.CanExtract && facts.RequirementCount == 0:
		result.NextAction = OperationExtract
	case facts.ApprovedRequirements == 0:
		result.NextAction = "REVIEW_REQUIREMENTS"
	case result.Capabilities.CanGenerate && facts.TestCaseCount == 0:
		result.NextAction = OperationGenerate
	case result.Capabilities.CanPublish && facts.ReleaseCount == 0:
		result.NextAction = "PUBLISH_SUITE_RELEASE"
	default:
		result.NextAction = "REVIEW_WORKFLOW"
	}
	return result, nil
}

func workflowStep(key string, completed, total int, complete bool,
	reasons []BlockingReason, active Job,
) Step {
	state := "READY"
	if complete {
		state = "COMPLETE"
	}
	if len(reasons) > 0 && !complete {
		state = "BLOCKED"
	}
	if active.ID > 0 {
		if active.Status == StatusFailed || active.Status == StatusPartialFailed {
			state = "FAILED"
		} else if !Terminal(active.Status) {
			state = "IN_PROGRESS"
		}
		completed, total = active.CompletedUnits, active.TotalUnits
	}
	return Step{Key: key, State: state, CompletedUnits: completed,
		TotalUnits: total, BlockingReasons: reasons}
}

func stepJob(active, latest map[string]Job, operation string) Job {
	if item, ok := active[operation]; ok {
		return item
	}
	item := latest[operation]
	if item.Status == StatusFailed || item.Status == StatusPartialFailed {
		return item
	}
	return Job{}
}

func stepReasons(items []BlockingReason, step string) []BlockingReason {
	result := []BlockingReason{}
	for _, item := range items {
		if item.Step == step {
			result = append(result, item)
		}
	}
	return result
}

func roleAtLeast(actual, required string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	if actual == "" {
		// Authentication is optional in local/demo mode; authorization middleware
		// enforces the real role whenever an auth token is configured.
		actual = "admin"
	}
	ranks := map[string]int{"viewer": 1, "editor": 2, "reviewer": 3, "admin": 4}
	return ranks[actual] >= ranks[required]
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func budgetAvailable(facts readFacts) bool {
	return facts.BudgetRemainingTokens > 0 &&
		(!facts.CostBudgetConfigured || facts.BudgetRemainingCost > 0)
}

func classifyError(err error) (string, bool) {
	switch {
	case errors.Is(err, testcase.ErrGenerationLease):
		return "GENERATION_LEASE_LOST", false
	case errors.Is(err, testcase.ErrProposalConflict):
		return "GENERATION_BASELINE_CHANGED", false
	case errors.Is(err, ErrInputStale), errors.Is(err, requirement.ErrStaleIndex):
		return "INPUT_SNAPSHOT_STALE", false
	case errors.Is(err, requirement.ErrNoIndex), errors.Is(err, testcase.ErrNoApprovedSource),
		errors.Is(err, document.ErrNoParsedDocuments), errors.Is(err, document.ErrSourceSnapshotBlocked):
		return "WORKFLOW_PRECONDITION_FAILED", false
	case errors.Is(err, context.Canceled), errors.Is(err, ErrCanceled):
		return "CANCELED", false
	default:
		return "WORKFLOW_PROCESSING_FAILED", true
	}
}
