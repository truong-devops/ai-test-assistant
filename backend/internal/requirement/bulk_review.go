package requirement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ReviewSelection struct {
	ID           int64  `json:"id"`
	ExpectedHash string `json:"expected_hash"`
}
type BulkReviewInput struct {
	Items        []ReviewSelection `json:"items"`
	Decision     string            `json:"decision"`
	ReviewerName string            `json:"reviewer_name"`
	Comment      string            `json:"comment"`
}
type ReviewResult struct {
	ID            int64  `json:"id"`
	RequirementID int64  `json:"requirement_id,omitempty"`
	Status        string `json:"status"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// Each item commits its audit and receipt atomically. A crash mid-batch leaves
// successful decisions replayable, while failures remain visible per revision.
func (s *Service) BulkReview(ctx context.Context, setID int64, input BulkReviewInput, key, actor string) ([]ReviewResult, error) {
	if setID <= 0 || len(input.Items) == 0 || len(input.Items) > 100 || len(key) == 0 || len(key) > 160 || strings.TrimSpace(actor) == "" || len(input.Comment) > 4000 || len(input.ReviewerName) > 160 || (input.Decision != DecisionApproved && input.Decision != DecisionRejected) {
		return nil, ErrInvalidInput
	}
	seen := map[int64]bool{}
	for _, item := range input.Items {
		if item.ID <= 0 || len(item.ExpectedHash) != 64 || seen[item.ID] {
			return nil, ErrInvalidInput
		}
		seen[item.ID] = true
	}
	request, _ := json.Marshal(struct {
		Input BulkReviewInput
		Actor string
	}{input, actor})
	var exists bool
	if err := s.repository.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_sets WHERE id=$1)`, setID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	result, err := s.repository.pool.Exec(ctx, `INSERT INTO requirement_review_commands(document_set_id,idempotency_key,request,result,actor)
 VALUES($1,$2,$3,'[]',$4) ON CONFLICT(document_set_id,idempotency_key) DO UPDATE SET request=requirement_review_commands.request
 WHERE requirement_review_commands.request=EXCLUDED.request`, setID, "batch:"+key, request, actor)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() != 1 {
		return nil, ErrIdempotencyConflict
	}
	results := make([]ReviewResult, 0, len(input.Items))
	for i, item := range input.Items {
		detail, err := s.repository.Review(ctx, item.ID, ReviewInput{DocumentSetID: setID, ExpectedHash: item.ExpectedHash,
			CommandKey: fmt.Sprintf("batch:%s:%d", key, i), ReviewerName: actor, Decision: input.Decision,
			Comment: input.Comment + "\nDisplay name: " + input.ReviewerName})
		row := ReviewResult{ID: item.ID, Status: "APPLIED"}
		if err == nil {
			row.RequirementID = detail.Requirement.ID
		} else {
			row.Status = "BLOCKED"
			switch {
			case errors.Is(err, ErrNotFound):
				row.Code = "NOT_FOUND"
				row.Message = "Yêu cầu không thuộc bộ tài liệu này."
			case errors.Is(err, ErrRevisionConflict):
				row.Code = "STALE_REVISION"
				row.Message = "Phiên bản hoặc bằng chứng đã đổi; xem và chọn lại."
			case errors.Is(err, ErrReviewBlocked):
				row.Code = "REVIEW_BLOCKED"
				row.Message = "Cần nguồn hợp lệ và giải quyết conflict/TBD trước khi duyệt."
			case errors.Is(err, ErrIdempotencyConflict):
				return nil, err
			default:
				return results, err
			}
		}
		results = append(results, row)
	}
	return results, nil
}
