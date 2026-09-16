package testcase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

type llmStub struct {
	output  string
	request llm.Request
}

func (s *llmStub) Generate(_ context.Context, request llm.Request) (llm.Response, error) {
	s.request = request
	return llm.Response{ID: "response-1", Model: "fixture", Output: s.output}, nil
}

const validTestCaseResponse = `{"test_cases":[{"title":"Đặt hàng hợp lệ","test_type":"HAPPY","risk":"HIGH","actor":"Khách hàng","precondition":"Đã đăng nhập","test_data":"Đơn hợp lệ","expected_result":"Đơn hàng được tạo","postcondition":"Đơn tồn tại","automation_status":"AUTOMATABLE","confidence":0.9,"assumptions":["Tài khoản thử nghiệm tồn tại"],"steps":[{"action":"Xác nhận đơn","expected_result":"Đơn hàng được tạo"}]}]}`

func TestLLMGeneratorGroundsExpectedResultAndLabelsAssumption(t *testing.T) {
	provider := &llmStub{output: validTestCaseResponse}
	generator := NewLLMGenerator(provider, "fixture", "fixture", 2000)
	items, call, err := generator.Generate(context.Background(), requirement.Detail{
		Requirement: requirement.Requirement{ID: 8, DocumentSetID: 3, RequirementKey: "UC-B08",
			VersionNumber: 1, Statement: "Đơn hàng được tạo", Status: requirement.StatusApproved},
		Evidence: []requirement.Evidence{{DocumentName: "URD", VersionNumber: 1,
			SourceLocator: "word/body/p[1]", Excerpt: "Đơn hàng được tạo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].GeneratedBy != "AI" || items[0].RequirementIDs[0] != 8 ||
		!strings.HasPrefix(items[0].Assumptions[0], "ASSUMPTION:") || call.Status != "COMPLETED" {
		t.Fatalf("items=%+v call=%+v", items, call)
	}
	if !strings.Contains(provider.request.Input, "<UNTRUSTED_APPROVED_EVIDENCE>") {
		t.Fatalf("prompt lacks untrusted evidence boundary: %s", provider.request.Input)
	}
}

func TestLLMGeneratorRejectsInventedExpectedResult(t *testing.T) {
	provider := &llmStub{output: strings.Replace(validTestCaseResponse,
		"Đơn hàng được tạo", "Hệ thống gửi email và hoàn tiền", 2)}
	generator := NewLLMGenerator(provider, "fixture", "fixture", 2000)
	_, call, err := generator.Generate(context.Background(), requirement.Detail{
		Requirement: requirement.Requirement{ID: 8, DocumentSetID: 3,
			RequirementKey: "UC-B08", Statement: "Đơn hàng được tạo", Status: requirement.StatusApproved},
		Evidence: []requirement.Evidence{{Excerpt: "Đơn hàng được tạo"}},
	})
	if !errors.Is(err, ErrInvalidProviderOutput) || call.Status != "INVALID_OUTPUT" {
		t.Fatalf("error=%v call=%+v", err, call)
	}
}
