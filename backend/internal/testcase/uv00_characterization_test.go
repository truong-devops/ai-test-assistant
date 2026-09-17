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

// These characterization tests deliberately record defects present at the UV-00
// baseline. UV-02 must replace their expectations with the new family/content
// identity contract; they are not the desired long-term behavior.
func TestUV00CharacterizationDistinctNegativeScenariosShareLogicalKey(t *testing.T) {
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
	if logicalCaseKey(left) != logicalCaseKey(right) {
		t.Fatalf("baseline changed: distinct scenarios no longer collide; replace this characterization in UV-02")
	}
}

func TestUV00CharacterizationGenerationKeyIgnoresStepAndTestDataChanges(t *testing.T) {
	fixture := loadUV00VersioningFixture(t)
	before, after := fixture.StepDataChange.Before, fixture.StepDataChange.After
	for _, proposal := range []*Proposal{&before, &after} {
		proposal.RequirementKeys = []string{fixture.RequirementKey}
		proposal.RequirementIDs = []int64{42}
	}
	if before.TestData == after.TestData || len(before.Steps) == len(after.Steps) {
		t.Fatalf("fixture does not change test data and steps: before=%+v after=%+v", before, after)
	}
	if generationKey(before) != generationKey(after) {
		t.Fatalf("baseline changed: step/data changes now affect generation key; replace this characterization in UV-02")
	}
}
