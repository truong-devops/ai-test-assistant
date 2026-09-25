package testcase

import (
	"errors"
	"time"
)

var (
	ErrProposalConflict = errors.New("proposal was already decided or its generation baseline changed")
	ErrGenerationLease  = errors.New("generation proposal attempt is no longer current")
)

type GenerationTarget struct {
	FamilyID   int64  `json:"family_id"`
	RevisionID int64  `json:"revision_id"`
	HeadToken  string `json:"head_token"`
}

type GenerationProposal struct {
	WorkflowUnitID   *int64               `json:"workflow_unit_id,omitempty"`
	ID               int64                `json:"id"`
	DocumentSetID    int64                `json:"document_set_id"`
	TestSuiteID      int64                `json:"test_suite_id"`
	WorkflowJobID    int64                `json:"workflow_job_id"`
	ProposalKey      string               `json:"proposal_key"`
	Classification   string               `json:"classification"`
	Reason           string               `json:"reason"`
	Content          RevisionContent      `json:"content"`
	ContentHash      string               `json:"content_hash"`
	Generation       GenerationProvenance `json:"generation"`
	Candidates       []GenerationTarget   `json:"candidates"`
	SourceRevision   int64                `json:"source_revision"`
	Status           string               `json:"status"`
	ResultRevisionID *int64               `json:"result_revision_id,omitempty"`
	Decision         string               `json:"decision"`
	DecisionReason   string               `json:"decision_reason"`
	DecidedBy        string               `json:"decided_by"`
	DecidedAt        *time.Time           `json:"decided_at,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
}

type ProposalDecision struct {
	Decision               string `json:"decision"` // CREATE_NEW, REVISE, KEEP, ARCHIVE, DISMISS
	TargetFamilyID         int64  `json:"target_family_id,omitempty"`
	ExpectedHeadRevisionID int64  `json:"expected_head_revision_id,omitempty"`
	Reason                 string `json:"reason"`
}

type ProposalPage struct {
	Proposals  []GenerationProposal `json:"proposals"`
	NextBefore int64                `json:"next_before,omitempty"`
}

type ProposalComparison struct {
	Target GenerationTarget `json:"target"`
	Before RevisionContent  `json:"before"`
	After  RevisionContent  `json:"after"`
}
