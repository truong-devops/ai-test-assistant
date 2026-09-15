package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

type repairStoreStub struct {
	subject      RepairSubject
	completion   *RepairCompletion
	unrepairable string
}

func (s *repairStoreStub) LoadRepairSubject(context.Context, RepairJob) (RepairSubject, error) {
	return s.subject, nil
}
func (s *repairStoreStub) CompleteRepair(_ context.Context, _ RepairJob, _ RepairSubject,
	completion RepairCompletion,
) (RepairJob, error) {
	s.completion = &completion
	return RepairJob{Status: RepairWaitingReview}, nil
}
func (s *repairStoreStub) MarkRepairUnrepairable(_ context.Context, _ RepairJob, reason,
	_, _, _, _ string, _, _ int, _ int64,
) error {
	s.unrepairable = reason
	return nil
}

type repairProviderStub struct {
	response llm.Response
	err      error
}

func (s repairProviderStub) Generate(context.Context, llm.Request) (llm.Response, error) {
	return s.response, s.err
}

func repairFixture() (RepairJob, RepairSubject) {
	expected := strings.Repeat("a", 64)
	source := "package cart\n\nimport \"testing\"\n\nfunc TestOrder(t *testing.T) { missing(t) }\n"
	job := RepairJob{ID: 7, TestRunItemID: 8, QueueAttemptCount: 1,
		ExpectedResultHash: expected, MaxOutputTokens: 1000, MaxCostMicroUSD: 100000,
		AllowedChangePolicy: repairPolicy}
	subject := RepairSubject{Job: job, ActualResult: "undefined: missing",
		Artifact: Artifact{ID: 3, TestCaseID: 42, Framework: FrameworkGoTest,
			FilePath: "internal/cart/cart_test.go", Source: source, SourceHash: hash([]byte(source)),
			ExpectedResultHash: expected, Assertions: []string{"Order is created"},
			BusinessContext:  json.RawMessage(`{"expected_result":"created"}`),
			TechnicalContext: json.RawMessage(`[]`), TestCaseSnapshot: json.RawMessage(`{}`)}}
	return job, subject
}

func repairResponse(subject RepairSubject, assertions []string, code string) llm.Response {
	proposal := proposedArtifact{Framework: FrameworkGoTest, TargetFile: subject.Artifact.FilePath,
		PackageName: "cart", Setup: "fixture", Assertions: assertions,
		ExpectedResultHash: subject.Artifact.ExpectedResultHash,
		TestCaseIDs:        []int64{subject.Artifact.TestCaseID}, Code: code}
	output, _ := json.Marshal(proposal)
	return llm.Response{ID: "response-1", Model: "fixture", Output: string(output),
		Usage: llm.Usage{InputTokens: 100, OutputTokens: 50}}
}

func TestRepairProcessorCreatesIndependentDraftCandidate(t *testing.T) {
	job, subject := repairFixture()
	store := &repairStoreStub{subject: subject}
	response := repairResponse(subject, []string{"  order  IS created "},
		"package cart\n\nimport \"testing\"\n\nfunc TestOrder(t *testing.T) { if got := 1; got != 1 { t.Fatal(got) } }\n")
	processor := NewRepairProcessor(store, repairProviderStub{response: response}, "test", "fixture", 1, 2)
	if err := processor.Process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if store.completion == nil || store.completion.AfterSourceHash == subject.Artifact.SourceHash {
		t.Fatalf("repair completion=%+v", store.completion)
	}
	if store.unrepairable != "" {
		t.Fatalf("unexpected unrepairable result: %s", store.unrepairable)
	}
}

func TestRepairProcessorRejectsBusinessAssertionChange(t *testing.T) {
	job, subject := repairFixture()
	store := &repairStoreStub{subject: subject}
	response := repairResponse(subject, []string{"Any result is acceptable"},
		"package cart\nimport \"testing\"\nfunc TestOrder(t *testing.T) { if false { t.Fatal() } }\n")
	processor := NewRepairProcessor(store, repairProviderStub{response: response}, "test", "fixture", 0, 0)
	if err := processor.Process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(store.unrepairable, "semantic assertions") || store.completion != nil {
		t.Fatalf("guardrail result=%q completion=%+v", store.unrepairable, store.completion)
	}
}

func TestRepairProcessorMarksUnchangedCandidateUnrepairable(t *testing.T) {
	job, subject := repairFixture()
	store := &repairStoreStub{subject: subject}
	response := repairResponse(subject, subject.Artifact.Assertions, subject.Artifact.Source)
	processor := NewRepairProcessor(store, repairProviderStub{response: response}, "test", "fixture", 0, 0)
	if err := processor.Process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(store.unrepairable, "unchanged") || store.completion != nil {
		t.Fatalf("guardrail result=%q completion=%+v", store.unrepairable, store.completion)
	}
}

func TestRepairProcessorRetriesProviderFailure(t *testing.T) {
	job, subject := repairFixture()
	store := &repairStoreStub{subject: subject}
	providerErr := errors.New("temporary provider failure")
	processor := NewRepairProcessor(store, repairProviderStub{err: providerErr}, "test", "fixture", 0, 0)
	err := processor.Process(context.Background(), job)
	if !errors.Is(err, providerErr) || store.unrepairable != "" || store.completion != nil {
		t.Fatalf("error=%v unrepairable=%q completion=%+v", err, store.unrepairable, store.completion)
	}
}

func TestRepairProcessorEnforcesPerJobCostBudget(t *testing.T) {
	job, subject := repairFixture()
	job.MaxCostMicroUSD = 1
	store := &repairStoreStub{subject: subject}
	response := repairResponse(subject, subject.Artifact.Assertions,
		"package cart\nimport \"testing\"\nfunc TestOrder(t *testing.T) { if false { t.Fatal() } }\n")
	processor := NewRepairProcessor(store, repairProviderStub{response: response}, "test", "fixture", 1, 2)
	if err := processor.Process(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(store.unrepairable, "exceeds budget") || store.completion != nil {
		t.Fatalf("guardrail result=%q completion=%+v", store.unrepairable, store.completion)
	}
}

func TestSemanticAssertionComparisonIgnoresOrderCaseAndSpacing(t *testing.T) {
	if !sameSemanticAssertions([]string{"A   happens", "B happens"},
		[]string{" b HAPPENS ", "a happens"}) {
		t.Fatal("equivalent assertions rejected")
	}
	if sameSemanticAssertions([]string{"A happens"}, []string{"A does not happen"}) {
		t.Fatal("changed assertion accepted")
	}
}
