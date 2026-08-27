package review

import "time"

const (
	DecisionAccepted = "ACCEPTED"
	DecisionRejected = "REJECTED"

	DefaultReviewerName = "local-reviewer"
	MaxReviewerNameBytes = 128
	MaxCommentBytes = 4000
)

type Review struct {
	ID              int64     `json:"id"`
	GeneratedTestID int64     `json:"generated_test_id"`
	ReviewerName    string    `json:"reviewer_name"`
	Decision        string    `json:"decision"`
	Comment         string    `json:"comment"`
	CreatedAt       time.Time `json:"created_at"`
}

type DecisionInput struct {
	ReviewerName string `json:"reviewer_name"`
	Comment      string `json:"comment"`
}
