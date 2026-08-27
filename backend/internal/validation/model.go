package validation

import "time"

const (
	StatusPassed   = "PASSED"
	StatusFailed   = "FAILED"
	StatusTimedOut = "TIMED_OUT"

	DefaultMaxRepositoryFiles = 10_000
	DefaultMaxRepositoryBytes = 100 << 20
	DefaultMaxOutputBytes     = 1 << 20
)

type Run struct {
	ID              int64     `json:"id"`
	AnalysisJobID   int64     `json:"analysis_job_id"`
	GeneratedTestID int64     `json:"generated_test_id"`
	AttemptNumber   int       `json:"attempt_number"`
	Command         string    `json:"command"`
	Status          string    `json:"status"`
	ExitCode        int       `json:"exit_code"`
	DurationMS      int64     `json:"duration_ms"`
	Stdout          string    `json:"stdout"`
	Stderr          string    `json:"stderr"`
	OutputTruncated bool      `json:"output_truncated"`
	CreatedAt       time.Time `json:"created_at"`
}

type SandboxRequest struct {
	Workspace string
	Command   []string
}

type SandboxResult struct {
	ExitCode        int
	Duration        time.Duration
	Stdout          string
	Stderr          string
	TimedOut        bool
	OutputTruncated bool
}
