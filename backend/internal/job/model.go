package job

import (
	"encoding/json"
	"time"
)

const (
	StatusPending         = "PENDING"
	StatusFetchingSource  = "FETCHING_SOURCE"
	StatusAnalyzingChange = "ANALYZING_CHANGE"
	StatusFailed          = "FAILED"
)

type AnalysisJob struct {
	ID              int64           `json:"id"`
	ProjectID       int64           `json:"project_id"`
	MergeRequestIID int64           `json:"merge_request_iid"`
	SourceSHA       string          `json:"source_sha"`
	TargetSHA       string          `json:"target_sha"`
	SourceBranch    string          `json:"source_branch"`
	TargetBranch    string          `json:"target_branch"`
	Title           string          `json:"title"`
	WebURL          string          `json:"web_url"`
	Status          string          `json:"status"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	WebhookUUID     string          `json:"webhook_uuid"`
	AttemptCount    int             `json:"attempt_count"`
	RawEvent        json.RawMessage `json:"-"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

type EnqueueInput struct {
	ProjectID       int64
	MergeRequestIID int64
	SourceSHA       string
	WebhookUUID     string
	RawEvent        json.RawMessage
}

type MergeRequestMetadata struct {
	SourceSHA    string
	TargetSHA    string
	SourceBranch string
	TargetBranch string
	Title        string
	WebURL       string
}

type ChangedFile struct {
	ID            int64  `json:"id"`
	AnalysisJobID int64  `json:"analysis_job_id"`
	OldPath       string `json:"old_path"`
	NewPath       string `json:"new_path"`
	ChangeType    string `json:"change_type"`
	Additions     int    `json:"additions"`
	Deletions     int    `json:"deletions"`
	Diff          string `json:"diff"`
	NewFile       bool   `json:"new_file"`
	RenamedFile   bool   `json:"renamed_file"`
	DeletedFile   bool   `json:"deleted_file"`
	Collapsed     bool   `json:"collapsed"`
	TooLarge      bool   `json:"too_large"`
}
