package testcase

import (
	"encoding/json"
	"os"
	"testing"
)

type uv00VersioningFixture struct {
	RequirementKey       string     `json:"requirement_key"`
	DistinctNegativeCase []Proposal `json:"distinct_negative_cases"`
	StepDataChange       struct {
		Before Proposal `json:"before"`
		After  Proposal `json:"after"`
	} `json:"step_data_change"`
}

func loadUV00VersioningFixture(t *testing.T) uv00VersioningFixture {
	t.Helper()
	payload, err := os.ReadFile("testdata/uv00/versioning-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture uv00VersioningFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func proposalContent(proposal Proposal) RevisionContent {
	content := RevisionContent{Title: proposal.Title, TestType: proposal.TestType,
		Risk: proposal.Risk, Actor: proposal.Actor, Precondition: proposal.Precondition,
		TestData: proposal.TestData, ExpectedResult: proposal.ExpectedResult,
		Postcondition: proposal.Postcondition, Assumptions: proposal.Assumptions,
		RequirementRevisionIDs: proposal.RequirementIDs}
	for _, step := range proposal.Steps {
		content.Steps = append(content.Steps, StepInput{Action: step.Action,
			ExpectedResult: step.ExpectedResult})
	}
	return content
}

func TestUV02DistinctNegativeScenariosHaveDistinctCanonicalContent(t *testing.T) {
	fixture := loadUV00VersioningFixture(t)
	if len(fixture.DistinctNegativeCase) != 2 {
		t.Fatalf("fixture negative cases=%d", len(fixture.DistinctNegativeCase))
	}
	for index := range fixture.DistinctNegativeCase {
		fixture.DistinctNegativeCase[index].RequirementKeys = []string{fixture.RequirementKey}
		fixture.DistinctNegativeCase[index].RequirementIDs = []int64{42}
	}
	left, right := fixture.DistinctNegativeCase[0], fixture.DistinctNegativeCase[1]
	if left.Title == right.Title || left.TestData == right.TestData ||
		left.Steps[0].ExpectedResult == right.Steps[0].ExpectedResult {
		t.Fatalf("fixture no longer represents two distinct scenarios: left=%+v right=%+v", left, right)
	}
	leftHash, err := revisionContentHash(proposalContent(left))
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := revisionContentHash(proposalContent(right))
	if err != nil {
		t.Fatal(err)
	}
	if leftHash == rightHash {
		t.Fatal("distinct negative scenarios must not share canonical content identity")
	}
}

func TestUV02StepAndTestDataChangesAffectCanonicalContent(t *testing.T) {
	fixture := loadUV00VersioningFixture(t)
	before, after := fixture.StepDataChange.Before, fixture.StepDataChange.After
	for _, proposal := range []*Proposal{&before, &after} {
		proposal.RequirementKeys = []string{fixture.RequirementKey}
		proposal.RequirementIDs = []int64{42}
	}
	if before.TestData == after.TestData || len(before.Steps) == len(after.Steps) {
		t.Fatalf("fixture does not change test data and steps: before=%+v after=%+v", before, after)
	}
	beforeHash, err := revisionContentHash(proposalContent(before))
	if err != nil {
		t.Fatal(err)
	}
	afterHash, err := revisionContentHash(proposalContent(after))
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash == afterHash {
		t.Fatal("step and test-data changes must affect the canonical fingerprint")
	}
}
