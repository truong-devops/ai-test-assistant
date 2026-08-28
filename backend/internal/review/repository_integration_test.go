//go:build integration

package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

func TestRepositoryAggregatesLatestReviewDecisions(t *testing.T) {
	ctx, pool := reviewTestPool(t)
	defer pool.Close()
	projectID, analysisID, staleID, currentID, secondID := createReviewFixture(t, ctx, pool, "aggregate")
	defer deleteReviewProject(pool, projectID)
	repository := NewRepository(pool)

	if _, err := repository.Decide(ctx, staleID, DecisionAccepted, "Minh", "old version"); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("stale Decide() error=%v", err)
	}
	first, err := repository.Decide(ctx, currentID, DecisionAccepted, "Minh", "ready to merge")
	if err != nil || first.Decision != DecisionAccepted || first.Comment != "ready to merge" {
		t.Fatalf("first review=%#v error=%v", first, err)
	}
	assertReviewAnalysisStatus(t, ctx, pool, analysisID, job.StatusWaitingReview, false)
	if _, err := repository.Decide(ctx, currentID, DecisionAccepted, "Minh", "again"); !errors.Is(err, ErrAlreadyReviewed) {
		t.Fatalf("duplicate Decide() error=%v", err)
	}
	second, err := repository.Decide(ctx, secondID, DecisionRejected, "Lan", "assertion is too weak")
	if err != nil || second.Decision != DecisionRejected {
		t.Fatalf("second review=%#v error=%v", second, err)
	}
	assertReviewAnalysisStatus(t, ctx, pool, analysisID, job.StatusRejected, true)
	reviews, err := repository.List(ctx, analysisID)
	if err != nil || len(reviews) != 2 || reviews[0].GeneratedTestID != currentID ||
		reviews[1].GeneratedTestID != secondID {
		t.Fatalf("reviews=%#v error=%v", reviews, err)
	}
}

func TestRepositoryAcceptsAnalysisWhenAllLatestTestsAccepted(t *testing.T) {
	ctx, pool := reviewTestPool(t)
	defer pool.Close()
	projectID, analysisID, _, currentID, secondID := createReviewFixture(t, ctx, pool, "accepted")
	defer deleteReviewProject(pool, projectID)
	repository := NewRepository(pool)
	if _, err := repository.Decide(ctx, currentID, DecisionAccepted, "Reviewer", "good"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Decide(ctx, secondID, DecisionAccepted, "Reviewer", "good"); err != nil {
		t.Fatal(err)
	}
	assertReviewAnalysisStatus(t, ctx, pool, analysisID, job.StatusAccepted, true)
}

func TestRepositoryAllowsOnlyOneConcurrentDecision(t *testing.T) {
	ctx, pool := reviewTestPool(t)
	defer pool.Close()
	projectID, analysisID, _, currentID, _ := createSingleReviewFixture(t, ctx, pool, "concurrent")
	defer deleteReviewProject(pool, projectID)
	repository := NewRepository(pool)

	start := make(chan struct{})
	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, decision := range []string{DecisionAccepted, DecisionRejected} {
		waitGroup.Add(1)
		go func(decision string) {
			defer waitGroup.Done()
			<-start
			_, err := repository.Decide(context.Background(), currentID, decision, "Reviewer", decision)
			results <- err
		}(decision)
	}
	close(start)
	waitGroup.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrAlreadyReviewed) && !errors.Is(err, ErrNotReady) {
			t.Fatalf("concurrent Decide() error=%v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent decisions=%d, want 1", successes)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM test_reviews WHERE generated_test_id=$1`, currentID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("review count=%d error=%v", count, err)
	}
	analysis, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || (analysis.Status != job.StatusAccepted && analysis.Status != job.StatusRejected) {
		t.Fatalf("analysis=%#v error=%v", analysis, err)
	}
}

func reviewTestPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool
}

func createReviewFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	suffix string,
) (projectID, analysisID, staleID, currentID, secondID int64) {
	t.Helper()
	projectID, analysisID, firstRecommendationID, secondRecommendationID := createReviewAnalysis(t, ctx, pool, suffix)
	staleID = insertReviewGeneratedTest(t, ctx, pool, analysisID, firstRecommendationID, 1, "stale")
	currentID = insertReviewGeneratedTest(t, ctx, pool, analysisID, firstRecommendationID, 2, "current")
	secondID = insertReviewGeneratedTest(t, ctx, pool, analysisID, secondRecommendationID, 1, "second")
	return projectID, analysisID, staleID, currentID, secondID
}

func createSingleReviewFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	suffix string,
) (projectID, analysisID, staleID, currentID, secondID int64) {
	t.Helper()
	projectID, analysisID, firstRecommendationID, _ := createReviewAnalysis(t, ctx, pool, suffix)
	currentID = insertReviewGeneratedTest(t, ctx, pool, analysisID, firstRecommendationID, 1, "only")
	return projectID, analysisID, 0, currentID, 0
}

func createReviewAnalysis(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	suffix string,
) (projectID, analysisID, firstRecommendationID, secondRecommendationID int64) {
	t.Helper()
	if err := pool.QueryRow(ctx, `INSERT INTO projects
		(name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ($1,$2,'https://gitlab.example.com/review.git','main','go','active') RETURNING id`,
		"review-"+suffix, time.Now().UnixNano()).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO analysis_jobs
		(project_id, merge_request_iid, source_sha, target_sha, status, webhook_uuid)
		VALUES ($1,1,'head','base',$2,$3) RETURNING id`, projectID, job.StatusWaitingReview,
		fmt.Sprintf("review-%s-%d", suffix, time.Now().UnixNano())).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var fileID, symbolID int64
	if err := pool.QueryRow(ctx, `INSERT INTO changed_files
		(analysis_job_id, old_path, new_path, change_type, diff)
		VALUES ($1,'service.go','service.go','modified','+changed') RETURNING id`, analysisID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO changed_symbols
		(changed_file_id, symbol_name, symbol_kind, package_name, start_line, end_line, change_type, change_summary)
		VALUES ($1,'CreateUser','function','user',1,2,'modified','changed CreateUser') RETURNING id`, fileID).Scan(&symbolID); err != nil {
		t.Fatal(err)
	}
	for index, destination := range []*int64{&firstRecommendationID, &secondRecommendationID} {
		if err := pool.QueryRow(ctx, `INSERT INTO test_recommendations
			(analysis_job_id, changed_symbol_id, title, description, priority, rationale,
			 scenario, expected_behavior, model_name, prompt_version)
			VALUES ($1,$2,$3,'description','high','rationale','scenario','expected','fixture','recommend-test-v1') RETURNING id`,
			analysisID, symbolID, fmt.Sprintf("Recommendation %d", index+1)).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	return projectID, analysisID, firstRecommendationID, secondRecommendationID
}

func insertReviewGeneratedTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	analysisID, recommendationID int64, attempt int, marker string,
) int64 {
	t.Helper()
	code := fmt.Sprintf("package user\n\nimport \"testing\"\n\nfunc Test%s(t *testing.T) { t.Log(%q) }\n", marker, marker)
	var generatedID int64
	if err := pool.QueryRow(ctx, `INSERT INTO generated_tests
		(analysis_job_id, recommendation_id, file_path, test_names, code, code_hash,
		 model_name, prompt_version, generation_attempt)
		VALUES ($1,$2,$3,$4,$5,$6,'fixture','generate-test-v1',$7) RETURNING id`,
		analysisID, recommendationID, fmt.Sprintf("%s_generated_test.go", marker),
		fmt.Sprintf("[\"Test%s\"]", marker), code, generation.CodeHash(code), attempt).Scan(&generatedID); err != nil {
		t.Fatal(err)
	}
	return generatedID
}

func assertReviewAnalysisStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	analysisID int64, wantStatus string, wantFinished bool,
) {
	t.Helper()
	analysis, _, err := job.NewRepository(pool).Get(ctx, analysisID)
	if err != nil || analysis.Status != wantStatus || (analysis.FinishedAt != nil) != wantFinished {
		t.Fatalf("analysis=%#v error=%v want status=%s finished=%v", analysis, err, wantStatus, wantFinished)
	}
}

func deleteReviewProject(pool *pgxpool.Pool, projectID int64) {
	_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id=$1`, projectID)
}
