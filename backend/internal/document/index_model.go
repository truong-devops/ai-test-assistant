package document

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	IndexNotIndexed = "NOT_INDEXED"
	IndexIndexing   = "INDEXING"
	IndexReady      = "READY"
	IndexFailed     = "FAILED"

	FlowNone      = "NONE"
	FlowMain      = "MAIN"
	FlowAlternate = "ALTERNATE"
	FlowException = "EXCEPTION"

	VersionLatest         = "LATEST"
	VersionLatestApproved = "LATEST_APPROVED"
	VersionAllIndexed     = "ALL_INDEXED"

	SemanticChunkerVersion = "document-semantic-v3"
)

var (
	ErrIndexNotReady       = errors.New("document index is not ready")
	ErrNoParsedDocuments   = errors.New("document set has no parsed documents")
	ErrInvalidSearch       = errors.New("invalid document retrieval query")
	ErrUnapprovedEvidence  = errors.New("evidence document version is not approved")
	ErrSourceReviewBlocked = errors.New("document source review is blocked")
)

type IndexStatus struct {
	DocumentSetID       int64      `json:"document_set_id"`
	Status              string     `json:"status"`
	Generation          int64      `json:"generation"`
	InputFingerprint    string     `json:"input_fingerprint,omitempty"`
	VersionCount        int        `json:"version_count"`
	SkippedVersionCount int        `json:"skipped_version_count"`
	ChunkCount          int        `json:"chunk_count"`
	WarningCount        int        `json:"warning_count"`
	EmbeddingModel      string     `json:"embedding_model"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	RequestedAt         *time.Time `json:"requested_at,omitempty"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type IndexSource struct {
	Document Document
	Version  Version
	Blocks   []Block
}

type SemanticChunk struct {
	ID                int64           `json:"id"`
	DocumentSetID     int64           `json:"document_set_id"`
	DocumentVersionID int64           `json:"document_version_id"`
	DocumentBlockID   int64           `json:"document_block_id"`
	ChunkKey          string          `json:"chunk_key"`
	ParentChunkKey    string          `json:"parent_chunk_key"`
	ChunkType         string          `json:"chunk_type"`
	FlowType          string          `json:"flow_type"`
	Identifier        string          `json:"identifier"`
	Title             string          `json:"title"`
	Content           string          `json:"content"`
	RawContent        string          `json:"raw_content"`
	ContentHash       string          `json:"content_hash"`
	SourceLocator     string          `json:"source_locator"`
	EmbeddingModel    string          `json:"embedding_model"`
	Metadata          json.RawMessage `json:"metadata"`
	SourceBlockIDs    []int64         `json:"source_block_ids,omitempty"`
	Embedding         []float32       `json:"-"`
	ApprovalStatus    string          `json:"approval_status,omitempty"`
	ExactScore        float64         `json:"exact_score,omitempty"`
	LexicalScore      float64         `json:"lexical_score,omitempty"`
	SemanticScore     float64         `json:"semantic_score,omitempty"`
	AuthorityScore    float64         `json:"authority_score,omitempty"`
	Score             float64         `json:"score,omitempty"`
	CreatedAt         time.Time       `json:"created_at,omitempty"`
	UpdatedAt         time.Time       `json:"updated_at,omitempty"`
}

type RetrievalQuery struct {
	DocumentSetID int64  `json:"-"`
	Query         string `json:"query"`
	Identifier    string `json:"identifier,omitempty"`
	ChunkType     string `json:"chunk_type,omitempty"`
	FlowType      string `json:"flow_type,omitempty"`
	VersionPolicy string `json:"version_policy,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type ContextSnapshot struct {
	ID              int64           `json:"id"`
	DocumentSetID   int64           `json:"document_set_id"`
	Purpose         string          `json:"purpose"`
	QueryText       string          `json:"query_text"`
	VersionPolicy   string          `json:"version_policy"`
	IndexGeneration int64           `json:"index_generation"`
	EmbeddingModel  string          `json:"embedding_model"`
	RetrievalConfig json.RawMessage `json:"retrieval_config"`
	SnapshotHash    string          `json:"snapshot_hash"`
	CreatedAt       time.Time       `json:"created_at"`
	Items           []SemanticChunk `json:"items"`
}

type VersionReviewInput struct {
	ReviewerName string `json:"reviewer_name"`
	Decision     string `json:"decision"`
	Comment      string `json:"comment"`
}

type VersionReview struct {
	ID                int64     `json:"id"`
	DocumentVersionID int64     `json:"document_version_id"`
	ReviewerName      string    `json:"reviewer_name"`
	Decision          string    `json:"decision"`
	Comment           string    `json:"comment"`
	CreatedAt         time.Time `json:"created_at"`
}

type GoldenQueryResult struct {
	Query     string  `json:"query"`
	K         int     `json:"k"`
	Expected  int     `json:"expected"`
	Found     int     `json:"found"`
	RecallAtK float64 `json:"recall_at_k"`
}

func EvaluateRecallAtK(query string, expectedChunkIDs []int64, results []SemanticChunk, k int) GoldenQueryResult {
	if k < 0 {
		k = 0
	}
	if k > len(results) {
		k = len(results)
	}
	expected := make(map[int64]struct{}, len(expectedChunkIDs))
	for _, id := range expectedChunkIDs {
		expected[id] = struct{}{}
	}
	found := make(map[int64]struct{})
	for _, item := range results[:k] {
		if _, ok := expected[item.ID]; ok {
			found[item.ID] = struct{}{}
		}
	}
	recall := float64(0)
	if len(expected) > 0 {
		recall = float64(len(found)) / float64(len(expected))
	}
	return GoldenQueryResult{Query: query, K: k, Expected: len(expected), Found: len(found), RecallAtK: recall}
}
