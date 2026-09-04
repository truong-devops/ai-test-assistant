package impact

import (
	"context"
	"errors"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

const (
	ModeSSA         = "SSA"
	ModeASTFallback = "AST_FALLBACK"

	RelationCalls      = "CALLS"
	RelationImplements = "IMPLEMENTS"
	RelationUsesType   = "USES_TYPE"

	ReasonDirectChange            = "DIRECT_CHANGE"
	ReasonCaller                  = "CALLER"
	ReasonCallee                  = "CALLEE"
	ReasonInterfaceImplementation = "INTERFACE_IMPLEMENTATION"
	ReasonTypeUsage               = "TYPE_USAGE"
	ReasonExistingTest            = "EXISTING_TEST"
)

var ErrNotFound = errors.New("impact analysis not found")

type DirectSymbol struct {
	FilePath string
	Symbol   job.ChangedSymbol
}

type Result struct {
	SourceSHA      string `json:"source_sha"`
	Mode           string `json:"mode"`
	Algorithm      string `json:"algorithm"`
	MaxDepth       int    `json:"max_depth"`
	MaxNodes       int    `json:"max_nodes"`
	PackageCount   int    `json:"package_count"`
	FallbackReason string `json:"fallback_reason,omitempty"`
	Nodes          []Node `json:"nodes"`
	Edges          []Edge `json:"edges"`
}

type Run struct {
	ID             int64     `json:"id"`
	AnalysisJobID  int64     `json:"analysis_job_id"`
	ProjectID      int64     `json:"project_id"`
	SourceSHA      string    `json:"source_sha"`
	Mode           string    `json:"mode"`
	Algorithm      string    `json:"algorithm"`
	MaxDepth       int       `json:"max_depth"`
	MaxNodes       int       `json:"max_nodes"`
	PackageCount   int       `json:"package_count"`
	FallbackReason string    `json:"fallback_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Node struct {
	ID           int64    `json:"id,omitempty"`
	Key          string   `json:"stable_key"`
	PackagePath  string   `json:"package_path"`
	PackageName  string   `json:"package_name"`
	SymbolName   string   `json:"symbol_name"`
	ReceiverName string   `json:"receiver_name,omitempty"`
	SymbolKind   string   `json:"symbol_kind"`
	FilePath     string   `json:"file_path"`
	StartLine    int      `json:"start_line"`
	EndLine      int      `json:"end_line"`
	DirectChange bool     `json:"direct_change"`
	ExistingTest bool     `json:"existing_test"`
	Depth        int      `json:"depth"`
	Score        float64  `json:"score"`
	ReasonCodes  []string `json:"reason_codes"`
}

type Edge struct {
	ID         int64   `json:"id,omitempty"`
	FromKey    string  `json:"from_key,omitempty"`
	ToKey      string  `json:"to_key,omitempty"`
	FromNodeID int64   `json:"from_node_id,omitempty"`
	ToNodeID   int64   `json:"to_node_id,omitempty"`
	Relation   string  `json:"relation"`
	ReasonCode string  `json:"reason_code"`
	Depth      int     `json:"depth"`
	Score      float64 `json:"score"`
}

type Bundle struct {
	Run   Run    `json:"run"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type AnalysisGetter interface {
	Get(ctx context.Context, id int64) (job.AnalysisJob, []job.ChangedFile, error)
}

type Reader interface {
	Get(ctx context.Context, analysisID int64) (Bundle, error)
}

type Service struct {
	analyses AnalysisGetter
	reader   Reader
}

func NewService(analyses AnalysisGetter, reader Reader) *Service {
	return &Service{analyses: analyses, reader: reader}
}

func (s *Service) Get(ctx context.Context, analysisID int64) (Bundle, error) {
	if analysisID <= 0 {
		return Bundle{}, job.ErrNotFound
	}
	if _, _, err := s.analyses.Get(ctx, analysisID); err != nil {
		return Bundle{}, err
	}
	return s.reader.Get(ctx, analysisID)
}
