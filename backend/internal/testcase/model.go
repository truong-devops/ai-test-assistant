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
	ErrNotFound         = errors.New("test case not found")
	ErrInvalidInput     = errors.New("invalid test case input")
	ErrNoApprovedSource = errors.New("no approved requirement baseline is available")
	ErrReviewBlocked    = errors.New("test case cannot be approved")
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
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
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
	ReviewerName   string `json:"reviewer_name"`
	Decision       string `json:"decision"`
	Comment        string `json:"comment"`
	Title          string `json:"title,omitempty"`
	Precondition   string `json:"precondition,omitempty"`
	TestData       string `json:"test_data,omitempty"`
	ExpectedResult string `json:"expected_result,omitempty"`
	Postcondition  string `json:"postcondition,omitempty"`
	Risk           string `json:"risk,omitempty"`
}

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
	DocumentSetID       int64          `json:"document_set_id"`
	ApprovedDenominator int            `json:"approved_denominator"`
	CoveredCount        int            `json:"covered_count"`
	CoveragePercent     float64        `json:"coverage_percent"`
	BaselineComplete    bool           `json:"baseline_complete"`
	ConflictCount       int            `json:"conflict_count"`
	TBDCount            int            `json:"tbd_count"`
	RejectedCount       int            `json:"rejected_count"`
	DuplicateCount      int            `json:"duplicate_count"`
	UncoveredCount      int            `json:"uncovered_count"`
	Matrix              []CoverageCell `json:"matrix"`
}
