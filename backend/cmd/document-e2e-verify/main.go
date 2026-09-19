package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Count  int64  `json:"count"`
	Detail string `json:"detail"`
}

type report struct {
	Passed        bool      `json:"passed"`
	DocumentSetID int64     `json:"document_set_id"`
	ProjectID     int64     `json:"project_id"`
	AnalysisID    int64     `json:"analysis_id"`
	TestRunID     int64     `json:"test_run_id"`
	VerifiedAt    time.Time `json:"verified_at"`
	Checks        []check   `json:"checks"`
}

func main() {
	setID := flag.Int64("document-set-id", 0, "document set used by the demonstration")
	projectID := flag.Int64("project-id", 0, "GitHub/GitLab project used by the demonstration")
	analysisID := flag.Int64("analysis-id", 0, "analysis created by the real webhook")
	runID := flag.Int64("test-run-id", 0, "sandbox test run created from the analysis")
	databaseURL := flag.String("database-url", "", "PostgreSQL URL; defaults to DATABASE_URL or DATABASE_URL_FILE")
	flag.Parse()
	if *setID <= 0 || *projectID <= 0 || *analysisID <= 0 || *runID <= 0 {
		fatal("all four positive IDs are required")
	}
	if strings.TrimSpace(*databaseURL) == "" {
		*databaseURL = databaseSecret()
	}
	if strings.TrimSpace(*databaseURL) == "" {
		fatal("database URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		fatal(err.Error())
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		fatal(err.Error())
	}

	result := report{Passed: true, DocumentSetID: *setID, ProjectID: *projectID,
		AnalysisID: *analysisID, TestRunID: *runID, VerifiedAt: time.Now().UTC(), Checks: []check{}}
	queries := []struct {
		name, detail, query string
		args                []any
	}{
		{"approved parsed source", "At least one immutable source version is parsed and approved.", `SELECT count(*) FROM document_versions WHERE document_set_id=$1 AND parse_status='PARSED' AND approval_status='APPROVED'`, []any{*setID}},
		{"ready semantic index", "The document RAG generation is ready.", `SELECT count(*) FROM document_index_status WHERE document_set_id=$1 AND status='READY' AND generation>0`, []any{*setID}},
		{"real LLM extraction", "A non-deterministic provider completed requirement extraction.", `SELECT count(*) FROM document_ai_calls WHERE document_set_id=$1 AND phase='REQUIREMENT_EXTRACTION' AND status='COMPLETED' AND provider NOT IN ('','deterministic','disabled','none')`, []any{*setID}},
		{"approved requirements", "Human-approved requirements exist.", `SELECT count(*) FROM requirements WHERE document_set_id=$1 AND status='APPROVED'`, []any{*setID}},
		{"approved test cases", "Human-approved business test cases exist.", `SELECT count(*) FROM test_cases WHERE document_set_id=$1 AND status='APPROVED'`, []any{*setID}},
		{"active project baseline", "The project is bound to this approved document set.", `SELECT count(*) FROM project_document_baselines b JOIN projects p ON p.id=b.project_id WHERE b.project_id=$1 AND b.document_set_id=$2 AND p.pipeline_mode='DOCUMENT_DRIVEN'`, []any{*projectID, *setID}},
		{"webhook analysis snapshot", "A real webhook analysis captured this immutable baseline.", `SELECT count(*) FROM analysis_jobs a JOIN analysis_baseline_snapshots s ON s.analysis_job_id=a.id WHERE a.id=$1 AND a.project_id=$2 AND s.document_set_id=$3 AND a.webhook_uuid<>''`, []any{*analysisID, *projectID, *setID}},
		{"approved automation", "At least one generated automation artifact was approved for this analysis.", `SELECT count(*) FROM automation_artifacts a JOIN test_cases t ON t.id=a.test_case_id WHERE a.analysis_job_id=$1 AND t.document_set_id=$2 AND a.status='APPROVED'`, []any{*analysisID, *setID}},
		{"completed sandbox run", "The selected run completed with immutable items and a sandbox fingerprint.", `SELECT count(*) FROM test_runs r WHERE r.id=$1 AND r.analysis_job_id=$2 AND r.project_id=$3 AND r.status='COMPLETED' AND r.environment='docker' AND r.image_digest<>'' AND EXISTS(SELECT 1 FROM test_run_items i WHERE i.test_run_id=r.id)`, []any{*runID, *analysisID, *projectID}},
		{"execution evidence", "The run retained evidence for at least one executed item.", `SELECT count(*) FROM test_run_evidence e JOIN test_run_items i ON i.id=e.test_run_item_id WHERE i.test_run_id=$1`, []any{*runID}},
		{"XLSX report", "An XLSX report was exported from this exact run.", `SELECT count(*) FROM test_exports WHERE document_set_id=$1 AND test_run_id=$2 AND format='XLSX'`, []any{*setID, *runID}},
	}
	for _, item := range queries {
		var count int64
		if err = pool.QueryRow(ctx, item.query, item.args...).Scan(&count); err != nil {
			fatal(fmt.Sprintf("%s: %v", item.name, err))
		}
		passed := count > 0
		result.Checks = append(result.Checks, check{Name: item.name, Passed: passed,
			Count: count, Detail: item.detail})
		result.Passed = result.Passed && passed
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	fmt.Println(string(encoded))
	if !result.Passed {
		os.Exit(1)
	}
}

func databaseSecret() string {
	if value := strings.TrimSpace(os.Getenv("DATABASE_URL")); value != "" {
		return value
	}
	path := strings.TrimSpace(os.Getenv("DATABASE_URL_FILE"))
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		fatal(err.Error())
	}
	return strings.TrimSpace(string(content))
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
