//go:build integration

package testcase

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type uv08Provider struct {
	output      string
	onFirstCall func()
	calls       int
}

func (p *uv08Provider) Generate(_ context.Context, _ llm.Request) (llm.Response, error) {
	p.calls++
	if p.calls == 1 && p.onFirstCall != nil {
		p.onFirstCall()
	}
	return llm.Response{ID: "uv08-response", Model: "uv08-fixture", Output: p.output}, nil
}

func TestUV08PinnedGenerationPreservesDistinctScenariosAndHumanHead(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	f := createUV02Fixture(t, ctx, pool)
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, f.setID) }()
	repository := NewRepository(pool)
	requirements := requirement.NewRepository(pool)
	base := uv02Proposal(f, "Same title and expected, separate scenarios", "address=home")
	base.ExpectedResult = "An order is created"
	base.Steps = []Step{{Action: "Submit with home address", ExpectedResult: base.ExpectedResult}}
	other := base
	other.TestData = "address=office"
	other.Steps = []Step{{Action: "Submit with office address", ExpectedResult: base.ExpectedResult}}
	// An exact repeated provider item is suppressed; a different data/step is not.
	payload, err := json.Marshal(ProposedResponse{TestCases: []Proposal{base, other, base}})
	if err != nil {
		t.Fatal(err)
	}
	var lateID int64
	provider := &uv08Provider{output: string(payload), onFirstCall: func() {
		if err := pool.QueryRow(ctx, `INSERT INTO requirements(document_set_id,requirement_key,title,statement,requirement_type,flow_type,status,source_snapshot_id)
			SELECT document_set_id,'REQ-UV08-LATE',title,statement,requirement_type,flow_type,'APPROVED',source_snapshot_id FROM requirements WHERE id=$1 RETURNING id`, f.requirementID).Scan(&lateID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO requirement_evidence(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
			SELECT $2,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash FROM requirement_evidence WHERE requirement_id=$1`, f.requirementID, lateID); err != nil {
			t.Fatal(err)
		}
	}}
	service := NewServiceWithLLM(repository, requirements, NewLLMGenerator(provider, "fixture", "uv08-fixture", 2000), requirements)
	input := GenerationBaseline{RequirementIDs: []int64{f.requirementID}, WorkflowJobID: 101, InputHash: strings.Repeat("a", 64), SourceRevision: 1}
	first, err := service.GeneratePinned(ctx, f.setID, input)
	if err != nil || first.RequirementCount != 1 || first.CreatedCount != 2 || first.SuppressedCount != 1 || provider.calls != 1 {
		t.Fatalf("first=%+v calls=%d err=%v", first, provider.calls, err)
	}
	cases, err := repository.List(ctx, f.setID)
	if err != nil || len(cases) != 2 {
		t.Fatalf("cases=%d err=%v", len(cases), err)
	}
	for _, item := range cases {
		detail, err := repository.Get(ctx, item.ID)
		if err != nil || len(detail.Requirements) != 1 || detail.Requirements[0].RequirementID != f.requirementID || item.Status != StatusDraft {
			t.Fatalf("generation widened baseline or approved result: %+v %v", detail, err)
		}
		var provenance struct {
			Generation GenerationProvenance `json:"generation"`
		}
		if err := json.Unmarshal(item.Provenance, &provenance); err != nil {
			t.Fatal(err)
		}
		p := provenance.Generation
		if p.WorkflowJobID != 101 || p.WorkflowInputHash != input.InputHash || p.Model != "uv08-fixture" || p.PromptVersion != GenerationPromptVersion || len(p.PromptHash) != 64 || p.ResponseHash != hash(provider.output) || len(p.RequirementReviewHash) != 64 {
			t.Fatalf("missing generation provenance: %+v", p)
		}
	}
	// A human edit during subsequent generation must remain the family head.
	title := "Human title must not be overwritten"
	edited, err := repository.CreateRevision(ctx, cases[0].FamilyID, CreateRevisionInput{BaseRevisionID: cases[0].ID, ExpectedHeadRevisionID: cases[0].ID, Patch: &RevisionPatch{Title: &title}, Reason: "Human review"}, "uv08-human", "qa")
	if err != nil {
		t.Fatal(err)
	}
	input.WorkflowJobID = 102
	retry, err := service.GeneratePinned(ctx, f.setID, input)
	if err != nil || retry.CreatedCount != 0 || retry.ReusedCount != 2 || retry.SuppressedCount != 0 {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	family, err := repository.GetFamily(ctx, cases[0].FamilyID)
	if err != nil || family.LatestRevision.ID != edited.Revision.ID || family.LatestRevision.Title != title || family.RevisionCounter != 2 {
		t.Fatalf("human head changed: %+v %v", family, err)
	}
	// Identical scenario on a different approved requirement is not silently merged.
	input.RequirementIDs = []int64{f.requirementID, lateID}
	expanded, err := service.GeneratePinned(ctx, f.setID, input)
	if err != nil || expanded.CreatedCount != 2 || expanded.ReusedCount != 2 {
		t.Fatalf("cross-source generation=%+v err=%v", expanded, err)
	}
	var callCount, releaseCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM document_ai_calls WHERE document_set_id=$1`, f.setID).Scan(&callCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM test_suite_releases WHERE document_set_id=$1`, f.setID).Scan(&releaseCount); err != nil {
		t.Fatal(err)
	}
	if callCount != 4 || releaseCount != 0 {
		t.Fatalf("calls=%d releases=%d", callCount, releaseCount)
	}
}
