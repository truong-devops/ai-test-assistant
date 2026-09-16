//go:build integration

package requirement_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/knowledge"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

func TestDocumentDrivenWorkflowIndexExtractReviewGenerateAndAudit(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	store, err := document.NewLocalFileStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	documentRepository := document.NewRepository(pool)
	documentService := document.NewService(documentRepository, store)
	set, err := documentService.CreateSet(ctx, document.CreateSetInput{
		Name:        "workflow-integration-" + time.Now().Format("20060102150405.000000000"),
		ProductName: "Commerce", Scope: "UC-B08",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, set.ID) }()

	markdown := `# UC-B08 Đặt hàng

## Luồng chính

- Khách hàng xác nhận đơn hợp lệ

## Luồng thay thế 1

- Khách hàng đổi địa chỉ nhận hàng

## Luồng thay thế 2

- Khách hàng quay lại giỏ hàng

## Luồng ngoại lệ 1

- Từ chối khi thiếu thông tin nhận hàng bắt buộc

## Luồng ngoại lệ 2

- Từ chối khi đơn không còn hợp lệ lúc xác nhận

## Chi tiết cần làm rõ

- Ngưỡng phí vận chuyển chưa xác định?
`
	item, version, err := documentService.Upload(ctx, set.ID, document.UploadInput{
		DocumentName: "UC-B08", DocumentType: document.TypeRequirements, Filename: "uc-b08.md",
	}, strings.NewReader(markdown))
	if err != nil {
		t.Fatal(err)
	}
	indexRepository := document.NewIndexRepository(pool)
	if _, err := indexRepository.ReviewVersion(ctx, version.ID, document.VersionReviewInput{
		ReviewerName: "PO", Decision: document.ApprovalApproved,
	}); !errors.Is(err, document.ErrSourceReviewBlocked) {
		t.Fatalf("unparsed source approval error=%v, want review blocked", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE document_versions SET parse_status='PARSING',
		attempt_count=1,lease_expires_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, version.ID); err != nil {
		t.Fatal(err)
	}
	version.ParseStatus, version.AttemptCount = document.ParseParsing, 1
	processor := document.NewProcessor(store, document.NewStructuredParser(), documentRepository)
	if err := processor.Process(ctx, version); err != nil {
		t.Fatal(err)
	}
	if _, err := indexRepository.ReviewVersion(ctx, version.ID, document.VersionReviewInput{
		ReviewerName: "PO", Decision: document.ApprovalApproved, Comment: "Approved fixture",
	}); err != nil {
		t.Fatal(err)
	}
	embedder := knowledge.NewHashEmbeddingClient("hash-document-test", knowledge.EmbeddingDimensions)
	indexService := document.NewIndexService(indexRepository, embedder)
	indexed, err := indexService.Index(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Status != document.IndexReady || indexed.ChunkCount < 10 || indexed.WarningCount != 0 {
		t.Fatalf("index=%+v", indexed)
	}
	results, err := indexService.Retrieve(ctx, document.RetrievalQuery{DocumentSetID: set.ID,
		Query: "UC-B08", VersionPolicy: document.VersionLatestApproved, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	flowCounts := map[string]int{}
	for _, result := range results {
		if result.DocumentSetID != set.ID || result.SourceLocator == "" {
			t.Fatalf("retrieval crossed set or lost citation: %+v", result)
		}
		flowCounts[result.FlowType]++
	}
	if flowCounts[document.FlowMain] == 0 || flowCounts[document.FlowAlternate] < 2 ||
		flowCounts[document.FlowException] < 2 {
		t.Fatalf("UC-B08 flow retrieval=%v", flowCounts)
	}

	requirementRepository := requirement.NewRepository(pool)
	requirementService := requirement.NewService(requirementRepository, indexService,
		requirement.DeterministicExtractor{})
	extracted, err := requirementService.Extract(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if extracted.CreatedCount < 6 || extracted.OpenQuestionCount != 1 {
		t.Fatalf("extraction=%+v", extracted)
	}
	firstInventory, err := requirementService.List(ctx, requirement.Filter{DocumentSetID: set.ID})
	if err != nil {
		t.Fatal(err)
	}
	approved := 0
	for _, req := range firstInventory {
		if req.Status == requirement.StatusTBD {
			continue
		}
		if _, err := requirementService.Review(ctx, req.ID, requirement.ReviewInput{
			ReviewerName: "BA", Decision: requirement.DecisionApproved, Comment: "Matches approved source",
		}); err != nil {
			t.Fatalf("approve %s: %v", req.RequirementKey, err)
		}
		approved++
	}
	if approved < 5 {
		t.Fatalf("approved=%d inventory=%+v", approved, firstInventory)
	}
	secondExtraction, err := requirementService.Extract(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondExtraction.CreatedCount != 0 || secondExtraction.ReusedCount < 6 {
		t.Fatalf("retry was not idempotent: %+v", secondExtraction)
	}
	secondInventory, err := requirementService.List(ctx, requirement.Filter{DocumentSetID: set.ID})
	if err != nil || len(secondInventory) != len(firstInventory) {
		t.Fatalf("inventory changed after retry: before=%d after=%d err=%v", len(firstInventory), len(secondInventory), err)
	}
	for _, scenario := range []string{"đổi địa chỉ", "quay lại giỏ hàng", "thiếu thông tin nhận hàng", "đơn không còn hợp lệ"} {
		found := false
		for _, req := range secondInventory {
			if strings.Contains(strings.ToLower(req.Statement), scenario) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("inventory omitted UC-B08 scenario %q: %+v", scenario, secondInventory)
		}
	}

	testService := testcase.NewService(testcase.NewRepository(pool), requirementService)
	generated, err := testService.Generate(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if generated.CreatedCount < approved {
		t.Fatalf("generation=%+v approved=%d", generated, approved)
	}
	cases, err := testService.List(ctx, set.ID)
	if err != nil || len(cases) == 0 {
		t.Fatalf("test cases=%d err=%v", len(cases), err)
	}
	detail, err := testService.Get(ctx, cases[0].ID)
	if err != nil || len(detail.Requirements) == 0 || len(detail.Evidence) == 0 {
		t.Fatalf("test detail lost source: %+v err=%v", detail, err)
	}
	coverage, err := testService.Coverage(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.ApprovedDenominator != approved || coverage.CoveredCount != approved ||
		coverage.TBDCount != 1 || coverage.BaselineComplete {
		t.Fatalf("coverage=%+v", coverage)
	}
	retryGeneration, err := testService.Regenerate(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retryGeneration.CreatedCount != 0 {
		t.Fatalf("regeneration created duplicate cases: %+v", retryGeneration)
	}

	var snapshotCountBefore, snapshotCountAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM document_context_snapshot_items i
		JOIN document_context_snapshots s ON s.id=i.context_snapshot_id WHERE s.document_set_id=$1`,
		set.ID).Scan(&snapshotCountBefore); err != nil {
		t.Fatal(err)
	}
	unchanged, err := indexService.Index(ctx, set.ID)
	if err != nil || unchanged.Generation != indexed.Generation {
		t.Fatalf("unchanged re-index=%+v err=%v", unchanged, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM document_context_snapshot_items i
		JOIN document_context_snapshots s ON s.id=i.context_snapshot_id WHERE s.document_set_id=$1`,
		set.ID).Scan(&snapshotCountAfter); err != nil {
		t.Fatal(err)
	}
	if snapshotCountAfter != snapshotCountBefore {
		t.Fatalf("historical snapshot changed during re-index: before=%d after=%d", snapshotCountBefore, snapshotCountAfter)
	}
	if _, err := indexRepository.ReviewVersion(ctx, version.ID, document.VersionReviewInput{
		ReviewerName: "PO", Decision: document.ApprovalRejected,
	}); !errors.Is(err, document.ErrSourceReviewBlocked) {
		t.Fatalf("source with approved requirements was revoked: %v", err)
	}
	_, _, _, err = documentService.GetVersion(ctx, item.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestApprovalGuardsRejectUncitedRequirementAndTestCase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var setID, requirementID, suiteID, caseID int64
	if err := pool.QueryRow(ctx, `INSERT INTO document_sets(name) VALUES($1) RETURNING id`,
		"approval-guard-"+time.Now().Format("20060102150405.000000000")).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM document_sets WHERE id=$1`, setID) }()
	if err := pool.QueryRow(ctx, `INSERT INTO requirements
		(document_set_id,requirement_key,title,statement,requirement_type)
		VALUES($1,'REQ-UNCITED','Uncited','No citation','FUNCTIONAL') RETURNING id`, setID).Scan(&requirementID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE requirements SET status='APPROVED' WHERE id=$1`, requirementID); err == nil {
		t.Fatal("uncited requirement was approved")
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_suites(document_set_id,name) VALUES($1,'Guard') RETURNING id`, setID).Scan(&suiteID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases
		(test_suite_id,document_set_id,test_case_key,title,test_type,expected_result,expected_result_hash)
		VALUES($1,$2,'TC-UNCITED','Uncited','HAPPY','Expected',$3) RETURNING id`,
		suiteID, setID, strings.Repeat("a", 64)).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE test_cases SET status='APPROVED' WHERE id=$1`, caseID); err == nil {
		t.Fatal("unlinked test case was approved")
	}
}
