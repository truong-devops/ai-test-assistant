package testcase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

type canonicalEvidenceRef struct {
	RequirementRevisionID int64  `json:"requirement_revision_id"`
	DocumentVersionID     int64  `json:"document_version_id"`
	DocumentBlockID       int64  `json:"document_block_id"`
	SourceLocator         string `json:"source_locator"`
	ExcerptHash           string `json:"excerpt_hash"`
}

type canonicalRevisionPayload struct {
	Title                  string                 `json:"title"`
	TestType               string                 `json:"test_type"`
	Risk                   string                 `json:"risk"`
	Actor                  string                 `json:"actor"`
	Precondition           string                 `json:"precondition"`
	TestData               string                 `json:"test_data"`
	Steps                  []StepInput            `json:"steps"`
	ExpectedResult         string                 `json:"expected_result"`
	Postcondition          string                 `json:"postcondition"`
	Assumptions            []string               `json:"assumptions"`
	RequirementRevisionIDs []int64                `json:"requirement_revision_ids"`
	EvidenceRefs           []canonicalEvidenceRef `json:"evidence_refs"`
	SourceSnapshotID       *int64                 `json:"source_snapshot_id"`
}

// Limit new editor payloads without making historical revisions unreadable.
func validateRevisionSize(input RevisionContent) error {
	if len(input.Steps) > 200 || len(input.RequirementRevisionIDs) > 200 || len(input.EvidenceRefs) > 500 || len(input.Assumptions) > 100 {
		return &ValidationError{Field: "steps", Message: "Tối đa 200 bước/requirements, 500 citations và 100 assumptions."}
	}
	for field, value := range map[string]string{"title": input.Title, "actor": input.Actor, "precondition": input.Precondition, "test_data": input.TestData, "expected_result": input.ExpectedResult, "postcondition": input.Postcondition} {
		if len(value) > 16000 {
			return &ValidationError{Field: field, Message: "Nội dung mỗi trường tối đa 16000 bytes."}
		}
	}
	for _, step := range input.Steps {
		if len(step.Action) > 16000 || len(step.ExpectedResult) > 16000 {
			return &ValidationError{Field: "steps", Message: "Nội dung mỗi bước tối đa 16000 bytes."}
		}
	}
	return nil
}

func normalizeRevisionContent(input RevisionContent) (RevisionContent, error) {
	input.Title = canonicalText(input.Title)
	input.TestType = strings.ToUpper(strings.TrimSpace(input.TestType))
	input.Risk = strings.ToUpper(strings.TrimSpace(input.Risk))
	input.Actor = canonicalText(input.Actor)
	input.Precondition = canonicalText(input.Precondition)
	input.TestData = canonicalText(input.TestData)
	input.ExpectedResult = canonicalText(input.ExpectedResult)
	input.Postcondition = canonicalText(input.Postcondition)
	if input.Title == "" || input.ExpectedResult == "" || !validTestType(input.TestType) ||
		input.Risk != "LOW" && input.Risk != "MEDIUM" && input.Risk != "HIGH" {
		return RevisionContent{}, ErrInvalidInput
	}
	if input.Steps == nil {
		input.Steps = []StepInput{}
	}
	for index := range input.Steps {
		input.Steps[index].Action = canonicalText(input.Steps[index].Action)
		input.Steps[index].ExpectedResult = canonicalText(input.Steps[index].ExpectedResult)
		if input.Steps[index].Action == "" {
			return RevisionContent{}, ErrInvalidInput
		}
	}
	if input.Assumptions == nil {
		input.Assumptions = []string{}
	}
	for index := range input.Assumptions {
		input.Assumptions[index] = canonicalText(input.Assumptions[index])
	}
	sort.Strings(input.Assumptions)
	input.Assumptions = uniqueStrings(input.Assumptions)
	input.RequirementRevisionIDs = uniqueSortedIDs(input.RequirementRevisionIDs)
	if len(input.RequirementRevisionIDs) == 0 {
		return RevisionContent{}, ErrNoApprovedSource
	}
	if input.EvidenceRefs == nil {
		input.EvidenceRefs = []EvidenceRef{}
	}
	for index := range input.EvidenceRefs {
		input.EvidenceRefs[index].SourceLocator = canonicalText(input.EvidenceRefs[index].SourceLocator)
		input.EvidenceRefs[index].ExcerptHash = strings.ToLower(strings.TrimSpace(input.EvidenceRefs[index].ExcerptHash))
	}
	sort.Slice(input.EvidenceRefs, func(i, j int) bool {
		left, right := input.EvidenceRefs[i], input.EvidenceRefs[j]
		if left.RequirementRevisionID != right.RequirementRevisionID {
			return left.RequirementRevisionID < right.RequirementRevisionID
		}
		if left.DocumentVersionID != right.DocumentVersionID {
			return left.DocumentVersionID < right.DocumentVersionID
		}
		if left.DocumentBlockID != right.DocumentBlockID {
			return left.DocumentBlockID < right.DocumentBlockID
		}
		return left.RequirementEvidenceID < right.RequirementEvidenceID
	})
	return input, nil
}

func revisionContentHash(input RevisionContent) (string, error) {
	input, err := normalizeRevisionContent(input)
	if err != nil {
		return "", err
	}
	evidence := make([]canonicalEvidenceRef, len(input.EvidenceRefs))
	for index, item := range input.EvidenceRefs {
		evidence[index] = canonicalEvidenceRef{RequirementRevisionID: item.RequirementRevisionID,
			DocumentVersionID: item.DocumentVersionID, DocumentBlockID: item.DocumentBlockID,
			SourceLocator: item.SourceLocator, ExcerptHash: item.ExcerptHash}
	}
	payload, err := json.Marshal(canonicalRevisionPayload{Title: input.Title,
		TestType: input.TestType, Risk: input.Risk, Actor: input.Actor,
		Precondition: input.Precondition, TestData: input.TestData, Steps: input.Steps,
		ExpectedResult: input.ExpectedResult, Postcondition: input.Postcondition,
		Assumptions: input.Assumptions, RequirementRevisionIDs: input.RequirementRevisionIDs,
		EvidenceRefs: evidence, SourceSnapshotID: input.SourceSnapshotID})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func revisionContentsEqual(left, right RevisionContent) bool {
	left, leftErr := normalizeRevisionContent(left)
	right, rightErr := normalizeRevisionContent(right)
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(left, right)
}

func applyRevisionPatch(base RevisionContent, patch RevisionPatch) RevisionContent {
	if patch.Title != nil {
		base.Title = *patch.Title
	}
	if patch.TestType != nil {
		base.TestType = *patch.TestType
	}
	if patch.Risk != nil {
		base.Risk = *patch.Risk
	}
	if patch.Actor != nil {
		base.Actor = *patch.Actor
	}
	if patch.Precondition != nil {
		base.Precondition = *patch.Precondition
	}
	if patch.TestData != nil {
		base.TestData = *patch.TestData
	}
	if patch.Steps != nil {
		base.Steps = append([]StepInput(nil), (*patch.Steps)...)
	}
	if patch.ExpectedResult != nil {
		base.ExpectedResult = *patch.ExpectedResult
	}
	if patch.Postcondition != nil {
		base.Postcondition = *patch.Postcondition
	}
	if patch.Assumptions != nil {
		base.Assumptions = append([]string(nil), (*patch.Assumptions)...)
	}
	if patch.RequirementRevisionIDs != nil {
		base.RequirementRevisionIDs = append([]int64(nil), (*patch.RequirementRevisionIDs)...)
	}
	if patch.EvidenceRefs != nil {
		base.EvidenceRefs = append([]EvidenceRef(nil), (*patch.EvidenceRefs)...)
	}
	if patch.SourceSnapshotID != nil {
		base.SourceSnapshotID = *patch.SourceSnapshotID
	}
	return base
}

func canonicalText(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"))
}
