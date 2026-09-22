//go:build integration

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUV08SelectedGenerationSnapshotOwnershipReplayAndFreshness(t *testing.T) {
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
	setID := workflowFixture(t, ctx, pool)
	foreignSet := workflowFixture(t, ctx, pool)
	defer cleanupWorkflowFixtures(t, pool, setID, foreignSet)
	var reqID, foreignID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM requirements WHERE document_set_id=$1`, setID).Scan(&reqID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM requirements WHERE document_set_id=$1`, foreignSet).Scan(&foreignID); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	service := NewService(repo, nil, nil, nil, 3)
	input := OperationInput{Operation: OperationGenerate, RequirementIDs: []int64{reqID}, RequestedBy: "QA"}
	job, created, err := service.Enqueue(ctx, setID, input, "uv08-selected", "editor")
	if err != nil || !created {
		t.Fatalf("enqueue=%+v %v", job, err)
	}
	var pinned generateInputSnapshot
	if json.Unmarshal(job.InputSnapshot, &pinned) != nil || len(pinned.Requirements) != 1 || pinned.Requirements[0].ID != reqID || len(pinned.SelectedRequirementIDs) != 1 {
		t.Fatalf("snapshot=%s", job.InputSnapshot)
	}
	allSnapshot, allHash, err := repo.BuildGenerateSnapshot(ctx, setID)
	if err != nil {
		t.Fatal(err)
	}
	var lateID int64
	if err := pool.QueryRow(ctx, `INSERT INTO requirements(document_set_id,requirement_key,title,statement,requirement_type,flow_type,status,source_snapshot_id)
		SELECT document_set_id,'REQ-UV08-LATE',title,statement,requirement_type,flow_type,'APPROVED',source_snapshot_id FROM requirements WHERE id=$1 RETURNING id`, reqID).Scan(&lateID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO requirement_evidence(requirement_id,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash)
		SELECT $2,document_set_id,document_version_id,document_block_id,source_locator,excerpt_hash FROM requirement_evidence WHERE requirement_id=$1`, reqID, lateID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateInputSnapshot(ctx, job); err != nil {
		t.Fatalf("unselected addition invalidated selected input: %v", err)
	}
	if err := repo.ValidateInputSnapshot(ctx, Job{Operation: OperationGenerate, DocumentSetID: setID, InputSnapshot: allSnapshot, InputHash: allHash}); !errors.Is(err, ErrInputStale) {
		t.Fatalf("legacy all-input freshness=%v", err)
	}
	replay, created, err := service.Enqueue(ctx, setID, input, "uv08-selected", "editor")
	if err != nil || created || replay.ID != job.ID {
		t.Fatalf("replay=%+v %v", replay, err)
	}
	if _, _, err := service.Enqueue(ctx, setID, OperationInput{Operation: OperationGenerate}, "uv08-selected", "editor"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("scope key collision=%v", err)
	}
	for _, ids := range [][]int64{{foreignID}, {reqID, foreignID}, {reqID, 9223372036854775807}} {
		_, _, err := repo.BuildGenerateSnapshotForRequirements(ctx, setID, ids)
		var blocked *BlockedError
		if !errors.As(err, &blocked) || blocked.Code != "GENERATION_SELECTION_BLOCKED" {
			t.Fatalf("selection=%v error=%v", ids, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE requirements SET status='DRAFT' WHERE id=$1`, reqID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateInputSnapshot(ctx, job); err == nil {
		t.Fatal("revoked selected approval was accepted")
	}
}
