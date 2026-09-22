package testcase

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

const (
	TypeHappy       = "HAPPY"
	TypeNegative    = "NEGATIVE"
	TypeBoundary    = "BOUNDARY"
	TypePermission  = "PERMISSION"
	TypeState       = "STATE"
	TypeIntegration = "INTEGRATION"
	TypeRegression  = "REGRESSION"
	TypeNFR         = "NFR"

	StatusDraft    = "DRAFT"
	StatusApproved = "APPROVED"
	StatusRejected = "REJECTED"
)

var (
	ErrNotFound            = errors.New("test case not found")
	ErrInvalidInput        = errors.New("invalid test case input")
	ErrNoApprovedSource    = errors.New("no approved requirement baseline is available")
	ErrReviewBlocked       = errors.New("test case cannot be approved")
	ErrRevisionConflict    = errors.New("test case family head changed")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with different input")
	ErrFamilyArchived      = errors.New("test case family is archived")
	ErrEvidenceInvalid     = errors.New("test case evidence is invalid or outside the family scope")
	ErrReleaseScope        = errors.New("suite release scope is incomplete or inconsistent")
)

type Suite struct {
	ID            int64     `json:"id"`
	DocumentSetID int64     `json:"document_set_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const (
	ReleaseScopeComplete = "COMPLETE"
	ReleaseScopePartial  = "PARTIAL"
)

type SuiteRelease struct {
	ID                       int64         `json:"id"`
	TestSuiteID              int64         `json:"test_suite_id"`
	DocumentSetID            int64         `json:"document_set_id"`
	ReleaseNumber            int           `json:"release_number"`
	Name                     string        `json:"name"`
	SourceSnapshotID         *int64        `json:"source_snapshot_id,omitempty"`
	ManifestHash             string        `json:"manifest_hash"`
	ScopeStatus              string        `json:"scope_status"`
	ApprovedRequirementCount int           `json:"approved_requirement_count"`
	CoveredRequirementCount  int           `json:"covered_requirement_count"`
	UncoveredRequirementIDs  []int64       `json:"uncovered_requirement_ids"`
	ScopeDecision            string        `json:"scope_decision"`
	PublishedBy              string        `json:"published_by"`
	Origin                   string        `json:"origin"`
	PublishedAt              time.Time     `json:"published_at"`
	CreatedAt                time.Time     `json:"created_at"`
	Items                    []ReleaseItem `json:"items"`
}

type ReleaseItem struct {
	ReleaseID          int64    `json:"release_id"`
	FamilyID           int64    `json:"family_id"`
	TestCaseID         int64    `json:"test_case_id"`
	Ordinal            int      `json:"ordinal"`
	PublicKey          string   `json:"public_key"`
	RevisionNumber     int      `json:"revision_number"`
	ContentHash        string   `json:"content_hash"`
	ExpectedResultHash string   `json:"expected_result_hash"`
	Revision           TestCase `json:"revision"`
}

type PublishReleaseInput struct {
	TestSuiteID      int64   `json:"test_suite_id"`
	SourceSnapshotID *int64  `json:"source_snapshot_id,omitempty"`
	RevisionIDs      []int64 `json:"revision_ids"`
	PublishedBy      string  `json:"published_by"`
	ScopeDecision    string  `json:"scope_decision,omitempty"`
}

type TestCase struct {
	ID                 int64           `json:"id"`
	TestSuiteID        int64           `json:"test_suite_id"`
	DocumentSetID      int64           `json:"document_set_id"`
	TestCaseKey        string          `json:"test_case_key"`
	VersionNumber      int             `json:"version_number"`
	Title              string          `json:"title"`
	TestType           string          `json:"test_type"`
	Risk               string          `json:"risk"`
	Actor              string          `json:"actor"`
	Precondition       string          `json:"precondition"`
	TestData           string          `json:"test_data"`
	ExpectedResult     string          `json:"expected_result"`
	ExpectedResultHash string          `json:"expected_result_hash"`
	Postcondition      string          `json:"postcondition"`
	Status             string          `json:"status"`
	AutomationStatus   string          `json:"automation_status"`
	Confidence         float64         `json:"confidence"`
	GeneratedBy        string          `json:"generated_by"`
	Assumptions        json.RawMessage `json:"assumptions"`
	GenerationKey      string          `json:"generation_key,omitempty"`
	SupersedesID       *int64          `json:"supersedes_test_case_id,omitempty"`
	FamilyID           int64           `json:"family_id"`
	ParentRevisionID   *int64          `json:"parent_revision_id,omitempty"`
	RestoredFromID     *int64          `json:"restored_from_revision_id,omitempty"`
	ContentHash        string          `json:"content_hash"`
	CreatedBy          string          `json:"created_by"`
	ChangeReason       string          `json:"change_reason"`
	SourceSnapshotID   *int64          `json:"source_snapshot_id,omitempty"`
	Provenance         json.RawMessage `json:"provenance"`
	SealedAt           *time.Time      `json:"sealed_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	LatestExecution    *ExecutionState `json:"latest_execution,omitempty"`
	NeedsSourceReview  bool            `json:"needs_source_review"`
}

type ExecutionState struct {
	TestRunID    int64     `json:"test_run_id"`
	Status       string    `json:"status"`
	ActualResult string    `json:"actual_result"`
	RunAt        time.Time `json:"run_at"`
}

type Step struct {
	ID             int64     `json:"id"`
	TestCaseID     int64     `json:"test_case_id"`
	Ordinal        int       `json:"ordinal"`
	Action         string    `json:"action"`
	ExpectedResult string    `json:"expected_result"`
	CreatedAt      time.Time `json:"created_at"`
}

type RequirementLink struct {
	TestCaseID       int64     `json:"test_case_id"`
	RequirementID    int64     `json:"requirement_id"`
	DocumentSetID    int64     `json:"document_set_id"`
	CoverageType     string    `json:"coverage_type"`
	RequirementKey   string    `json:"requirement_key,omitempty"`
	RequirementTitle string    `json:"requirement_title,omitempty"`
	FlowType         string    `json:"flow_type,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Review struct {
	ID           int64     `json:"id"`
	TestCaseID   int64     `json:"test_case_id"`
	ReviewerName string    `json:"reviewer_name"`
	Decision     string    `json:"decision"`
	Comment      string    `json:"comment"`
	ContentHash  string    `json:"content_hash"`
	Actor        string    `json:"actor"`
	CreatedAt    time.Time `json:"created_at"`
}

type Detail struct {
	TestCase     TestCase               `json:"test_case"`
	Steps        []Step                 `json:"steps"`
	Requirements []RequirementLink      `json:"requirements"`
	Evidence     []requirement.Evidence `json:"evidence"`
	Reviews      []Review               `json:"reviews"`
}

type Proposal struct {
	Title            string   `json:"title"`
	TestType         string   `json:"test_type"`
	Risk             string   `json:"risk"`
	Actor            string   `json:"actor"`
	Precondition     string   `json:"precondition"`
	TestData         string   `json:"test_data"`
	ExpectedResult   string   `json:"expected_result"`
	Postcondition    string   `json:"postcondition"`
	AutomationStatus string   `json:"automation_status"`
	Confidence       float64  `json:"confidence"`
	GeneratedBy      string   `json:"-"`
	Assumptions      []string `json:"assumptions"`
	Steps            []Step   `json:"steps"`
	RequirementIDs   []int64  `json:"-"`
	RequirementKeys  []string `json:"-"`
}

type GenerateSummary struct {
	DocumentSetID    int64 `json:"document_set_id"`
	SuiteID          int64 `json:"test_suite_id"`
	RequirementCount int   `json:"requirement_count"`
	CreatedCount     int   `json:"created_count"`
	ReusedCount      int   `json:"reused_count"`
	SuppressedCount  int   `json:"suppressed_count"`
}

type ReviewInput struct {
	ReviewerName        string `json:"reviewer_name"`
	Decision            string `json:"decision"`
	Comment             string `json:"comment"`
	Title               string `json:"title,omitempty"`
	Precondition        string `json:"precondition,omitempty"`
	TestData            string `json:"test_data,omitempty"`
	ExpectedResult      string `json:"expected_result,omitempty"`
	Postcondition       string `json:"postcondition,omitempty"`
	Risk                string `json:"risk,omitempty"`
	ExpectedContentHash string `json:"expected_content_hash,omitempty"`
}

type Family struct {
	ID                  int64     `json:"id"`
	TestSuiteID         int64     `json:"test_suite_id"`
	DocumentSetID       int64     `json:"document_set_id"`
	PublicKey           string    `json:"public_key"`
	LegacyKey           string    `json:"legacy_key,omitempty"`
	Archived            bool      `json:"archived"`
	RevisionCounter     int       `json:"revision_counter"`
	HeadRevisionID      *int64    `json:"head_revision_id,omitempty"`
	HeadToken           string    `json:"head_token"`
	NeedsIdentityReview bool      `json:"needs_identity_review"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	LatestRevision      *TestCase `json:"latest_revision,omitempty"`
	LatestApproved      *TestCase `json:"latest_approved_revision,omitempty"`
}

type StepInput struct {
	Action         string `json:"action"`
	ExpectedResult string `json:"expected_result"`
}

type EvidenceRef struct {
	RequirementRevisionID int64  `json:"requirement_revision_id"`
	RequirementEvidenceID int64  `json:"requirement_evidence_id"`
	DocumentVersionID     int64  `json:"document_version_id"`
	DocumentBlockID       int64  `json:"document_block_id"`
	SourceLocator         string `json:"source_locator"`
	ExcerptHash           string `json:"excerpt_hash"`
}

type RevisionContent struct {
	Title                  string        `json:"title"`
	TestType               string        `json:"test_type"`
	Risk                   string        `json:"risk"`
	Actor                  string        `json:"actor"`
	Precondition           string        `json:"precondition"`
	TestData               string        `json:"test_data"`
	Steps                  []StepInput   `json:"steps"`
	ExpectedResult         string        `json:"expected_result"`
	Postcondition          string        `json:"postcondition"`
	Assumptions            []string      `json:"assumptions"`
	RequirementRevisionIDs []int64       `json:"requirement_revision_ids"`
	EvidenceRefs           []EvidenceRef `json:"evidence_refs"`
	SourceSnapshotID       *int64        `json:"source_snapshot_id,omitempty"`
}

type RevisionPatch struct {
	Title                  *string        `json:"title,omitempty"`
	TestType               *string        `json:"test_type,omitempty"`
	Risk                   *string        `json:"risk,omitempty"`
	Actor                  *string        `json:"actor,omitempty"`
	Precondition           *string        `json:"precondition,omitempty"`
	TestData               *string        `json:"test_data,omitempty"`
	Steps                  *[]StepInput   `json:"steps,omitempty"`
	ExpectedResult         *string        `json:"expected_result,omitempty"`
	Postcondition          *string        `json:"postcondition,omitempty"`
	Assumptions            *[]string      `json:"assumptions,omitempty"`
	RequirementRevisionIDs *[]int64       `json:"requirement_revision_ids,omitempty"`
	EvidenceRefs           *[]EvidenceRef `json:"evidence_refs,omitempty"`
	SourceSnapshotID       **int64        `json:"source_snapshot_id,omitempty"`
}

type CreateRevisionInput struct {
	BaseRevisionID         int64            `json:"base_revision_id"`
	ExpectedHeadRevisionID int64            `json:"expected_head_revision_id"`
	ExpectedHeadToken      string           `json:"expected_head_token,omitempty"`
	Content                *RevisionContent `json:"content,omitempty"`
	Patch                  *RevisionPatch   `json:"patch,omitempty"`
	Reason                 string           `json:"reason"`
}

type RestoreInput struct {
	FromRevisionID         int64  `json:"from_revision_id"`
	ExpectedHeadRevisionID int64  `json:"expected_head_revision_id"`
	ExpectedHeadToken      string `json:"expected_head_token,omitempty"`
	Reason                 string `json:"reason"`
}

type ArchiveInput struct {
	Archived bool   `json:"archived"`
	Reason   string `json:"reason"`
}

type RevisionResult struct {
	Revision TestCase `json:"revision"`
	Created  bool     `json:"created"`
	Location string   `json:"location"`
}

type FieldDiff struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type RevisionDiff struct {
	FamilyID int64       `json:"family_id"`
	From     TestCase    `json:"from"`
	To       TestCase    `json:"to"`
	Changes  []FieldDiff `json:"changes"`
}

type RevisionConflictError struct {
	CurrentRevisionID int64
	CurrentHeadToken  string
}

func (e *RevisionConflictError) Error() string { return ErrRevisionConflict.Error() }
func (e *RevisionConflictError) Unwrap() error { return ErrRevisionConflict }

type BulkReviewInput struct {
	TestCaseIDs  []int64 `json:"test_case_ids"`
	ReviewerName string  `json:"reviewer_name"`
	Decision     string  `json:"decision"`
	Comment      string  `json:"comment"`
}

type CoverageCell struct {
	RequirementID  int64    `json:"requirement_id"`
	RequirementKey string   `json:"requirement_key"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	FlowType       string   `json:"flow_type"`
	TestCaseIDs    []int64  `json:"test_case_ids"`
	TestTypes      []string `json:"test_types"`
	Covered        bool     `json:"covered"`
	Warnings       []string `json:"warnings"`
}

type CoverageReport struct {
	DocumentSetID       int64           `json:"document_set_id"`
	ApprovedDenominator int             `json:"approved_denominator"`
	CoveredCount        int             `json:"covered_count"`
	CoveragePercent     float64         `json:"coverage_percent"`
	BaselineComplete    bool            `json:"baseline_complete"`
	ConflictCount       int             `json:"conflict_count"`
	TBDCount            int             `json:"tbd_count"`
	RejectedCount       int             `json:"rejected_count"`
	DuplicateCount      int             `json:"duplicate_count"`
	UncoveredCount      int             `json:"uncovered_count"`
	Layers              []CoverageLayer `json:"layers"`
	Matrix              []CoverageCell  `json:"matrix"`
}

type CoverageLayer struct {
	Key              string  `json:"key"`
	Label            string  `json:"label"`
	Numerator        int     `json:"numerator"`
	Denominator      int     `json:"denominator"`
	Percent          float64 `json:"percent"`
	SourceSnapshotID *int64  `json:"source_snapshot_id,omitempty"`
	SuiteReleaseID   *int64  `json:"suite_release_id,omitempty"`
	ReleaseNumber    *int    `json:"release_number,omitempty"`
}
