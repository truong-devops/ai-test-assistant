package requirement

import "time"

// Requirement is the document-grounded business statement. Phase 4 will add
// extraction and review behavior; Phase 1 defines the ownership boundary now
// so automation code cannot reuse legacy changed-symbol recommendations as
// business truth.
type Requirement struct {
	ID              int64     `json:"id"`
	DocumentSetID   int64     `json:"document_set_id"`
	RequirementKey  string    `json:"requirement_key"`
	VersionNumber   int       `json:"version_number"`
	Title           string    `json:"title"`
	Statement       string    `json:"statement"`
	RequirementType string    `json:"requirement_type"`
	FlowType        string    `json:"flow_type"`
	Actor           string    `json:"actor"`
	Precondition    string    `json:"precondition"`
	Postcondition   string    `json:"postcondition"`
	Priority        string    `json:"priority"`
	Status          string    `json:"status"`
	Confidence      float64   `json:"confidence"`
	SupersedesID    *int64    `json:"supersedes_requirement_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Evidence struct {
	ID                int64     `json:"id"`
	RequirementID     int64     `json:"requirement_id"`
	DocumentSetID     int64     `json:"document_set_id"`
	DocumentVersionID int64     `json:"document_version_id"`
	DocumentBlockID   int64     `json:"document_block_id"`
	SourceLocator     string    `json:"source_locator"`
	ExcerptHash       string    `json:"excerpt_hash"`
	CreatedAt         time.Time `json:"created_at"`
}
