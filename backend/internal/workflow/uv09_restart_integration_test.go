//go:build integration

package workflow

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/aibudget"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

// Fault barriers exist only in this test binary, never in the production worker.
func restartBarrier() { fmt.Println("UV09_KILL_BARRIER"); select {} }

type restartProvider struct{ mode string }

func (p restartProvider) Generate(context.Context, llm.Request) (llm.Response, error) {
	if p.mode == "in_provider" {
		restartBarrier()
	}
	return llm.Response{ID: "uv09-protocol-fixture", Model: "fixture", Usage: llm.Usage{InputTokens: 30, OutputTokens: 10, TotalTokens: 40},
		Output: `{"test_cases":[{"title":"Checkout","test_type":"HAPPY","risk":"LOW","actor":"Customer","precondition":"","test_data":"","expected_result":"Checkout creates an order","postcondition":"","automation_status":"MANUAL","confidence":1,"assumptions":[],"steps":[{"action":"Confirm checkout","expected_result":"Checkout creates an order"}]}]}`}, nil
}

type restartRecorder struct {
	repo *requirement.Repository
	mode string
}

func (r restartRecorder) SaveAICall(ctx context.Context, call requirement.AICall) error {
	if err := r.repo.SaveAICall(ctx, call); err != nil {
		return err
	}
	if r.mode == "before_commit" {
		restartBarrier()
	}
	return nil
}

type restartGenerator struct {
	service *testcase.Service
	mode    string
}

func (g restartGenerator) GeneratePinned(ctx context.Context, id int64, input testcase.GenerationBaseline) (testcase.GenerateSummary, error) {
	out, err := g.service.GeneratePinned(ctx, id, input)
	if err == nil && g.mode == "after_commit" {
		restartBarrier()
	}
	return out, err
}

type restartProcessor struct {
	service *Service
	jobID   int64
}

func (p restartProcessor) Process(ctx context.Context, job Job) (any, error) {
	if job.ID != p.jobID {
		return nil, fmt.Errorf("unexpected job %d; isolated queue required", job.ID)
	}
	return p.service.Process(ctx, job)
}

func TestUV09RestartHelper(t *testing.T) {
	mode := os.Getenv("UV09_RESTART_MODE")
	if mode == "" {
		t.Skip("subprocess helper only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	jobID, err := strconv.ParseInt(os.Getenv("UV09_RESTART_JOB"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	reqs := requirement.NewRepository(pool)
	generator := testcase.NewLLMGenerator(restartProvider{mode}, "fixture", "fixture", 512).ConfigureBudget(aibudget.NewManager(pool, 1, 2))
	cases := testcase.NewServiceWithLLM(testcase.NewRepository(pool), reqs, generator, restartRecorder{reqs, mode})
	repo := NewRepository(pool)
	service := NewService(repo, nil, nil, restartGenerator{cases, mode}, 3)
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), repo, restartProcessor{service, jobID}, WorkerOptions{
		PollInterval: time.Millisecond, RetryDelay: time.Millisecond, LeaseDuration: 600 * time.Millisecond, ProcessTimeout: 20 * time.Second})
	if err := worker.runOnce(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUV09WorkerSIGKILLBeforeAndAfterUnitCommit(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	for _, mode := range []string{"in_provider", "before_commit", "after_commit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			setID := workflowFixture(t, ctx, pool)
			// Retain evidence but never leave runnable fixture jobs behind after failure.
			defer func() {
				_, _ = pool.Exec(context.Background(), `UPDATE document_workflow_jobs SET status='CANCELED' WHERE document_set_id=$1 AND status IN ('QUEUED','RUNNING')`, setID)
			}()
			repo := NewRepository(pool)
			service := NewService(repo, nil, nil, nil, 3)
			var reqID int64
			if err = pool.QueryRow(ctx, `SELECT id FROM requirements WHERE document_set_id=$1`, setID).Scan(&reqID); err != nil {
				t.Fatal(err)
			}
			job, _, err := service.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate, ReviewProposals: true, GenerationScope: "SELECTED", RequirementIDs: []int64{reqID}}, "uv09-restart", "editor")
			if err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			child := exec.CommandContext(ctx, executable, "-test.run=^TestUV09RestartHelper$", "-test.v")
			child.Env = append(os.Environ(), "UV09_RESTART_MODE="+mode, fmt.Sprintf("UV09_RESTART_JOB=%d", job.ID))
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			child.Stderr = os.Stderr
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
			barrier := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if scanner.Text() == "UV09_KILL_BARRIER" {
						barrier <- true
						return
					}
				}
				barrier <- false
			}()
			select {
			case ok := <-barrier:
				if !ok {
					t.Fatal("child exited before kill barrier")
				}
			case <-ctx.Done():
				t.Fatal("kill barrier timeout")
			}
			claimed, err := repo.Get(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if claimed.Status != StatusRunning {
				t.Fatalf("not running at barrier: %+v", claimed)
			}
			if err = child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err = child.Wait(); err == nil {
				t.Fatal("worker exited normally instead of being killed")
			}
			var committed int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM test_case_generation_proposals WHERE workflow_job_id=$1`, job.ID).Scan(&committed); err != nil {
				t.Fatal(err)
			}
			wantBefore := 0
			if mode == "after_commit" {
				wantBefore = 1
			}
			if committed != wantBefore {
				t.Fatalf("proposal count at crash=%d want %d", committed, wantBefore)
			}
			if mode == "in_provider" {
				// Time travel applies only to this fixture's reservation expiry. The
				// actual worker crash/lease takeover uses SIGKILL and wall-clock expiry.
				if _, err = pool.Exec(ctx, `UPDATE document_ai_budget_reservations SET expires_at=NOW()-INTERVAL '1 second' WHERE workflow_job_id=$1`, job.ID); err != nil {
					t.Fatal(err)
				}
				manager := aibudget.NewManager(pool, 1, 2)
				held, err := manager.Status(ctx, setID)
				if err != nil || held.ReservedTokens <= 0 || held.UsedTokens != 0 || held.UnreconciledReservations != 1 {
					t.Fatalf("unknown usage lost: %+v %v", held, err)
				}
				read, err := repo.readFacts(ctx, setID)
				if err != nil || read.BudgetRemainingTokens != held.RemainingTokens {
					t.Fatalf("workflow budget disagrees with held usage: %+v %v", read, err)
				}
			}
			// Preserve a human decision made while the worker is dead and job still running.
			var reviewedID int64
			if mode == "after_commit" {
				if err = pool.QueryRow(ctx, `SELECT id FROM test_case_generation_proposals WHERE workflow_job_id=$1`, job.ID).Scan(&reviewedID); err != nil {
					t.Fatal(err)
				}
				cases := testcase.NewService(testcase.NewRepository(pool), requirement.NewRepository(pool))
				if _, err = cases.DecideProposal(ctx, reviewedID, testcase.ProposalDecision{Decision: "DISMISS", Reason: "Reviewed while worker was stopped"}, "uv09-human-decision", "qa"); err != nil {
					t.Fatal(err)
				}
			}
			for {
				var expired bool
				if err = pool.QueryRow(ctx, `SELECT lease_expires_at<=NOW() FROM document_workflow_jobs WHERE id=$1`, job.ID).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if expired {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("lease did not expire")
				case <-time.After(30 * time.Millisecond):
				}
			}
			restarted := exec.CommandContext(ctx, executable, "-test.run=^TestUV09RestartHelper$", "-test.v")
			restarted.Env = append(os.Environ(), "UV09_RESTART_MODE=resume", fmt.Sprintf("UV09_RESTART_JOB=%d", job.ID))
			if output, err := restarted.CombinedOutput(); err != nil {
				t.Fatalf("restart: %v\n%s", err, output)
			}
			done, err := repo.Get(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if done.Status != StatusSucceeded || done.AttemptCount != 2 || done.CompletedUnits != 1 {
				t.Fatalf("restart result=%+v", done)
			}
			var proposals, reservations, inputTokens, outputTokens, pending, unitAttempt, wrongAttribution, calls int
			err = pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM test_case_generation_proposals WHERE workflow_job_id=$1),
 (SELECT count(*) FROM document_ai_budget_reservations WHERE workflow_job_id=$1 AND status='COMPLETED'),
 (SELECT COALESCE(sum(input_tokens),0) FROM document_ai_budget_reservations WHERE workflow_job_id=$1),
 (SELECT COALESCE(sum(output_tokens),0) FROM document_ai_budget_reservations WHERE workflow_job_id=$1),
 (SELECT count(*) FROM document_ai_budget_reservations WHERE workflow_job_id=$1 AND status<>'COMPLETED'),
 (SELECT attempt_count FROM document_workflow_job_units WHERE workflow_job_id=$1 AND unit_key<>'operation'),
 (SELECT count(*) FROM document_ai_budget_reservations r JOIN document_workflow_job_units u ON u.id=r.workflow_unit_id WHERE r.workflow_job_id=$1 AND (u.workflow_job_id<>$1 OR u.unit_key='operation' OR r.workflow_attempt IS NULL)),
 (SELECT count(*) FROM document_ai_calls WHERE document_set_id=$2)`, job.ID, setID).Scan(&proposals, &reservations, &inputTokens, &outputTokens, &pending, &unitAttempt, &wrongAttribution, &calls)
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 2
			if mode == "after_commit" {
				wantCalls = 1
			}
			wantPending, wantUnitAttempt := 0, wantCalls
			if mode == "in_provider" {
				wantCalls, wantPending, wantUnitAttempt = 1, 1, 2
			}
			if proposals != 1 || reservations != wantCalls || calls != wantCalls || inputTokens != 30*wantCalls || outputTokens != 10*wantCalls || pending != wantPending || unitAttempt != wantUnitAttempt || wrongAttribution != 0 {
				t.Fatalf("proposals=%d completed reservations=%d calls=%d usage=%d/%d pending=%d unit attempts=%d wrong attribution=%d", proposals, reservations, calls, inputTokens, outputTokens, pending, unitAttempt, wrongAttribution)
			}
			budget, err := aibudget.NewManager(pool, 1, 2).Status(ctx, setID)
			if err != nil || budget.UsedTokens != int64(40*wantCalls) || budget.UsedCostMicroUSD != int64(50*wantCalls) || (mode != "in_provider" && budget.ReservedTokens != 0) || (mode == "in_provider" && (budget.ReservedTokens <= 0 || budget.UnreconciledReservations != 1)) {
				t.Fatalf("budget=%+v %v", budget, err)
			}
			if reviewedID > 0 {
				var status string
				if err = pool.QueryRow(ctx, `SELECT status FROM test_case_generation_proposals WHERE id=$1`, reviewedID).Scan(&status); err != nil || status != "DISMISSED" {
					t.Fatalf("human decision lost: %s %v", status, err)
				}
			}
			if err = repo.Heartbeat(ctx, claimed, time.Minute); err == nil {
				t.Fatal("dead attempt heartbeat accepted")
			}
			evidence, _ := json.Marshal(map[string]any{"mode": mode, "job": job.ID, "status": done.Status, "process_killed": true, "proposals": proposals, "provider_calls_with_recorded_usage": calls, "provider_invocations": wantUnitAttempt, "budget": budget})
			t.Log(string(evidence))
		})
	}
}
