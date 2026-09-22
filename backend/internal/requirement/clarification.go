package requirement

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ClarificationInput struct {
	Kind       string `json:"kind"`
	ID         int64  `json:"id"`
	Resolution string `json:"resolution"`
}
type ClarificationAudit struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	SubjectID  int64     `json:"subject_id"`
	Actor      string    `json:"actor"`
	Resolution string    `json:"resolution"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Service) ResolveClarification(ctx context.Context, setID int64, input ClarificationInput, actor string) error {
	input.Resolution = strings.TrimSpace(input.Resolution)
	if setID <= 0 || input.ID <= 0 || len(input.Resolution) < 10 || len(input.Resolution) > 8000 || actor == "" || (input.Kind != "CONFLICT" && input.Kind != "QUESTION") {
		return ErrInvalidInput
	}
	tx, err := s.repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM document_sets WHERE id=$1 FOR UPDATE`, setID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !active {
		return ErrReviewBlocked
	}
	var ids []int64
	var status, resolution string
	if input.Kind == "CONFLICT" {
		err = tx.QueryRow(ctx, `SELECT ARRAY[left_requirement_id,right_requirement_id],status,resolution FROM requirement_conflicts WHERE id=$1 AND document_set_id=$2 FOR UPDATE`, input.ID, setID).Scan(&ids, &status, &resolution)
	} else {
		err = tx.QueryRow(ctx, `SELECT array_remove(ARRAY[requirement_id],NULL),status,answer FROM open_questions WHERE id=$1 AND document_set_id=$2 FOR UPDATE`, input.ID, setID).Scan(&ids, &status, &resolution)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "OPEN" {
		if resolution == input.Resolution {
			return nil
		}
		return ErrRevisionConflict
	}
	if input.Kind == "CONFLICT" {
		_, err = tx.Exec(ctx, `UPDATE requirement_conflicts SET status='RESOLVED',resolution=$2,resolved_at=NOW() WHERE id=$1`, input.ID, input.Resolution)
	} else {
		_, err = tx.Exec(ctx, `UPDATE open_questions SET status='ANSWERED',answer=$2,answered_at=NOW() WHERE id=$1`, input.ID, input.Resolution)
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO requirement_clarification_audit(document_set_id,kind,subject_id,actor,resolution) VALUES($1,$2,$3,$4,$5)`, setID, input.Kind, input.ID, actor, input.Resolution); err != nil {
		return err
	}
	// Resolution is not approval. Keep the revision draft until explicit review.
	if _, err = tx.Exec(ctx, `UPDATE requirements r SET status='DRAFT',updated_at=NOW() WHERE id=ANY($1) AND status IN ('CONFLICT','TBD')
 AND NOT EXISTS(SELECT 1 FROM requirement_conflicts c WHERE c.status='OPEN' AND (c.left_requirement_id=r.id OR c.right_requirement_id=r.id))
 AND NOT EXISTS(SELECT 1 FROM open_questions q WHERE q.status='OPEN' AND q.requirement_id=r.id)`, ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) ClarificationHistory(ctx context.Context, setID int64) ([]ClarificationAudit, error) {
	rows, err := s.repository.pool.Query(ctx, `SELECT id,kind,subject_id,actor,resolution,created_at FROM requirement_clarification_audit WHERE document_set_id=$1 ORDER BY id DESC`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []ClarificationAudit{}
	for rows.Next() {
		var item ClarificationAudit
		if err = rows.Scan(&item.ID, &item.Kind, &item.SubjectID, &item.Actor, &item.Resolution, &item.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
