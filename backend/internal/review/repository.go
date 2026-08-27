package review

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{pool: pool} }

func (r *PostgresRepository) Decide(ctx context.Context, generatedTestID int64, decision, reviewerName, comment string) (Review, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Review{}, fmt.Errorf("begin review decision: %w", err)
	}
	defer tx.Rollback(ctx)

	const lockGenerated = `SELECT generated.analysis_job_id, generated.recommendation_id,
		generated.generation_attempt, analysis.status
		FROM generated_tests AS generated
		JOIN analysis_jobs AS analysis ON analysis.id=generated.analysis_job_id
		WHERE generated.id=$1
		FOR UPDATE OF generated, analysis`
	var analysisID, recommendationID int64
	var attempt int
	var analysisStatus string
	if err := tx.QueryRow(ctx, lockGenerated, generatedTestID).Scan(&analysisID, &recommendationID,
		&attempt, &analysisStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Review{}, job.ErrNotFound
		}
		return Review{}, fmt.Errorf("lock generated test for review: %w", err)
	}
	if analysisStatus != job.StatusWaitingReview {
		return Review{}, ErrNotReady
	}

	var newerExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM generated_tests
		WHERE recommendation_id=$1 AND generation_attempt>$2
	)`, recommendationID, attempt).Scan(&newerExists); err != nil {
		return Review{}, fmt.Errorf("check generated test version: %w", err)
	}
	if newerExists {
		return Review{}, ErrStaleVersion
	}

	var alreadyReviewed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM test_reviews WHERE generated_test_id=$1)`, generatedTestID).
		Scan(&alreadyReviewed); err != nil {
		return Review{}, fmt.Errorf("check existing review: %w", err)
	}
	if alreadyReviewed {
		return Review{}, ErrAlreadyReviewed
	}

	const insert = `INSERT INTO test_reviews (generated_test_id, reviewer_name, decision, comment)
		VALUES ($1,$2,$3,$4)
		RETURNING id, generated_test_id, reviewer_name, decision, comment, created_at`
	var result Review
	if err := tx.QueryRow(ctx, insert, generatedTestID, reviewerName, decision, comment).
		Scan(&result.ID, &result.GeneratedTestID, &result.ReviewerName, &result.Decision,
			&result.Comment, &result.CreatedAt); err != nil {
		return Review{}, fmt.Errorf("insert review decision: %w", err)
	}

	const aggregate = `WITH latest AS (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY recommendation_id ORDER BY generation_attempt DESC, id DESC
			) AS version_rank
			FROM generated_tests WHERE analysis_job_id=$1
		) AS versions WHERE version_rank=1
	), summary AS (
		SELECT COUNT(*) AS total,
			COUNT(reviews.id) AS reviewed,
			BOOL_OR(reviews.decision=$2) AS has_rejection
		FROM latest LEFT JOIN test_reviews AS reviews ON reviews.generated_test_id=latest.id
	)
	SELECT total, reviewed, COALESCE(has_rejection, FALSE) FROM summary`
	var total, reviewed int
	var hasRejection bool
	if err := tx.QueryRow(ctx, aggregate, analysisID, DecisionRejected).Scan(&total, &reviewed, &hasRejection); err != nil {
		return Review{}, fmt.Errorf("aggregate review decisions: %w", err)
	}
	if total > 0 && total == reviewed {
		nextStatus := job.StatusAccepted
		if hasRejection {
			nextStatus = job.StatusRejected
		}
		update, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, finished_at=NOW(),
			lease_expires_at=NULL, next_attempt_at=NOW(), error_message=NULL
			WHERE id=$1 AND status=$3`, analysisID, nextStatus, job.StatusWaitingReview)
		if err != nil {
			return Review{}, fmt.Errorf("complete review analysis: %w", err)
		}
		if update.RowsAffected() != 1 {
			return Review{}, ErrNotReady
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Review{}, fmt.Errorf("commit review decision: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) List(ctx context.Context, analysisID int64) ([]Review, error) {
	const query = `SELECT reviews.id, reviews.generated_test_id, reviews.reviewer_name,
		reviews.decision, reviews.comment, reviews.created_at
		FROM test_reviews AS reviews
		JOIN generated_tests AS generated ON generated.id=reviews.generated_test_id
		WHERE generated.analysis_job_id=$1
		ORDER BY reviews.created_at, reviews.id`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list review decisions: %w", err)
	}
	defer rows.Close()
	results := make([]Review, 0)
	for rows.Next() {
		var item Review
		if err := rows.Scan(&item.ID, &item.GeneratedTestID, &item.ReviewerName,
			&item.Decision, &item.Comment, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan review decision: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate review decisions: %w", err)
	}
	return results, nil
}
