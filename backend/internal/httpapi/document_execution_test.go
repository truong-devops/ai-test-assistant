package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/automation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/execution"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
)

type reportServiceStub struct {
	exportInput report.ExportInput
	download    report.ExportArtifact
}

type automationRepairServiceStub struct {
	requested automation.RepairRequest
	jobs      []automation.RepairJob
	err       error
}

func (s *automationRepairServiceStub) RequestRepair(_ context.Context, _ int64,
	input automation.RepairRequest,
) (automation.RepairJob, error) {
	s.requested = input
	return automation.RepairJob{ID: 11, Status: automation.RepairPending}, s.err
}
func (s *automationRepairServiceStub) ListRepairs(context.Context, int64) ([]automation.RepairJob, error) {
	return s.jobs, s.err
}

func TestAutomationRepairEndpointsAreAsynchronous(t *testing.T) {
	service := &automationRepairServiceStub{jobs: []automation.RepairJob{{ID: 11,
		Status: automation.RepairWaitingReview}}}
	request := httptest.NewRequest(http.MethodPost, "/api/test-run-items/9/repair",
		strings.NewReader(`{"requested_by":"QA"}`))
	request.SetPathValue("id", "9")
	response := httptest.NewRecorder()
	automationRepairHandler{service: service}.request(response, request)
	if response.Code != http.StatusAccepted || service.requested.RequestedBy != "QA" {
		t.Fatalf("status=%d request=%+v body=%s", response.Code, service.requested,
			response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/test-run-items/9/repairs", nil)
	request.SetPathValue("id", "9")
	response = httptest.NewRecorder()
	automationRepairHandler{service: service}.list(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "WAITING_REVIEW") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type executionServiceStub struct {
	run        execution.Run
	requested  execution.RequestInput
	classified execution.ClassificationInput
}

func (s *executionServiceStub) Get(context.Context, int64) (execution.Run, error) { return s.run, nil }
func (s *executionServiceStub) Request(_ context.Context, _ int64, input execution.RequestInput) (execution.Run, error) {
	s.requested = input
	return s.run, nil
}
func (s *executionServiceStub) ReviewClassification(_ context.Context, _ int64, input execution.ClassificationInput) (execution.Run, error) {
	s.classified = input
	return s.run, nil
}

func TestExecutionRequestAndClassificationEndpoints(t *testing.T) {
	service := &executionServiceStub{run: execution.Run{ID: 8}}
	request := httptest.NewRequest(http.MethodPost, "/api/test-runs/8/execute", strings.NewReader(`{"requested_by":"QA"}`))
	request.SetPathValue("id", "8")
	response := httptest.NewRecorder()
	executionHandler{service: service}.request(response, request)
	if response.Code != http.StatusAccepted || service.requested.RequestedBy != "QA" {
		t.Fatalf("request status=%d input=%+v", response.Code, service.requested)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/test-run-items/9/classification", strings.NewReader(`{"status":"PRODUCT_FAILED","reviewer_name":"Lead","reason":"actual mismatch"}`))
	request.SetPathValue("id", "9")
	response = httptest.NewRecorder()
	executionHandler{service: service}.classify(response, request)
	if response.Code != http.StatusOK || service.classified.Status != "PRODUCT_FAILED" || service.classified.Reason != "actual mismatch" {
		t.Fatalf("classification status=%d input=%+v", response.Code, service.classified)
	}
}

func (s *reportServiceStub) Export(_ context.Context, _ int64, input report.ExportInput) (report.ExportArtifact, error) {
	s.exportInput = input
	return report.ExportArtifact{ID: 9, DownloadURL: "/api/test-exports/9/download"}, nil
}
func (s *reportServiceStub) List(context.Context, int64) ([]report.ExportArtifact, error) {
	return []report.ExportArtifact{}, nil
}
func (s *reportServiceStub) Download(context.Context, int64) (report.ExportArtifact, error) {
	return s.download, nil
}

func TestReportExportEndpointPreservesFilterContract(t *testing.T) {
	service := &reportServiceStub{}
	request := httptest.NewRequest(http.MethodPost, "/api/document-sets/4/exports", strings.NewReader(`{"test_suite_id":7,"format":"XLSX","generated_by":"QA","test_case_ids":[12,13],"sort_by":"RISK"}`))
	request.SetPathValue("id", "4")
	response := httptest.NewRecorder()
	reportHandler{service: service}.export(response, request)
	if response.Code != http.StatusCreated || service.exportInput.TestSuiteID != 7 || len(service.exportInput.TestCaseIDs) != 2 || service.exportInput.SortBy != "RISK" {
		t.Fatalf("status=%d input=%+v body=%s", response.Code, service.exportInput, response.Body.String())
	}
	var body report.ExportArtifact
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.ID != 9 {
		t.Fatalf("body=%+v error=%v", body, err)
	}
}

func TestReportDownloadEndpointReturnsStoredBytesAndHash(t *testing.T) {
	service := &reportServiceStub{download: report.ExportArtifact{Filename: "safe-report.xlsx", ContentType: "application/test", ContentHash: strings.Repeat("a", 64), Content: []byte("workbook")}}
	request := httptest.NewRequest(http.MethodGet, "/api/test-exports/9/download", nil)
	request.SetPathValue("id", "9")
	response := httptest.NewRecorder()
	reportHandler{service: service}.download(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "workbook" || response.Header().Get("X-Content-SHA256") != strings.Repeat("a", 64) || response.Header().Get("Content-Disposition") != `attachment; filename="safe-report.xlsx"` {
		t.Fatalf("status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}
