package knowledge

import (
	"encoding/json"
	"time"
)

const (
	IndexStatusNotIndexed = "NOT_INDEXED"
	IndexStatusPending    = "PENDING"
	IndexStatusIndexing   = "INDEXING"
	IndexStatusReady      = "READY"
	IndexStatusFailed     = "FAILED"

	EmbeddingDimensions = 384
)

type IndexJob struct {
	ProjectID        int64      `json:"project_id"`
	Ref              string     `json:"ref"`
	Status           string     `json:"status"`
	Generation       int64      `json:"generation"`
	AttemptCount     int        `json:"attempt_count"`
	FileCount        int        `json:"file_count"`
	SkippedFileCount int        `json:"skipped_file_count"`
	ChunkCount       int        `json:"chunk_count"`
	EmbeddingModel   string     `json:"embedding_model"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	RequestedAt      time.Time  `json:"requested_at,omitzero"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at,omitzero"`
}

type DraftChunk struct {
	ChunkKey    string
	FilePath    string
	PackageName string
	SymbolName  string
	ChunkType   string
	Content     string
	StartLine   int
	EndLine     int
	Metadata    map[string]any
}

type KnowledgeChunk struct {
	ID              int64           `json:"id"`
	ProjectID       int64           `json:"project_id"`
	ChunkKey        string          `json:"chunk_key"`
	FilePath        string          `json:"file_path"`
	PackageName     string          `json:"package_name"`
	SymbolName      string          `json:"symbol_name"`
	ChunkType       string          `json:"chunk_type"`
	Content         string          `json:"content"`
	ContentHash     string          `json:"content_hash"`
	StartLine       int             `json:"start_line"`
	EndLine         int             `json:"end_line"`
	EmbeddingModel  string          `json:"embedding_model"`
	IndexRef        string          `json:"index_ref,omitempty"`
	IndexGeneration int64           `json:"index_generation,omitempty"`
	Metadata        json.RawMessage `json:"metadata"`
	Score           float64         `json:"score,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`

	Embedding []float32 `json:"-"`
}

type ChunkFingerprint struct {
	ContentHash    string
	EmbeddingModel string
}

type RetrievalQuery struct {
	ProjectID   int64
	Query       string
	PackageName string
	SymbolName  string
	FilePath    string
	PreferTests bool
	Limit       int
}
