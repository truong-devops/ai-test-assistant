package requirement

import (
	"errors"
	"strings"
	"testing"
)

const validRequirementResponse = `{"requirements":[{"identifier":"UC-B08","title":"Đặt hàng","statement":"Khách hàng xác nhận đơn hợp lệ","requirement_type":"USE_CASE","flow_type":"MAIN","actor":"Khách hàng","precondition":"Đã đăng nhập","postcondition":"Đơn được tạo","priority":"HIGH","risk":"HIGH","status":"DRAFT","confidence":0.91,"assumptions":[],"steps":[{"action":"Xác nhận","expected_result":"Đơn được tạo"}]}]}`

func TestParseResponseAcceptsStrictGroundedRequirement(t *testing.T) {
	result, err := ParseResponse(validRequirementResponse)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requirements) != 1 || result.Requirements[0].Identifier != "UC-B08" ||
		result.Requirements[0].Steps[0].Action != "Xác nhận" {
		t.Fatalf("result=%+v", result)
	}
}

func TestParseResponseRejectsUnsupportedStatusAndTrailingJSON(t *testing.T) {
	for _, input := range []string{
		strings.Replace(validRequirementResponse, `"status":"DRAFT"`, `"status":"APPROVED"`, 1),
		validRequirementResponse + `{}`,
		strings.Replace(validRequirementResponse, `"statement":"Khách hàng xác nhận đơn hợp lệ"`, `"statement":""`, 1),
	} {
		if _, err := ParseResponse(input); !errors.Is(err, ErrInvalidProviderOutput) {
			t.Fatalf("error=%v, want ErrInvalidProviderOutput", err)
		}
	}
}

func TestParseResponseNormalizesSafeProviderAliasesAndPercentConfidence(t *testing.T) {
	input := strings.NewReplacer(
		`"requirement_type":"USE_CASE"`, `"requirement_type":"user story"`,
		`"flow_type":"MAIN"`, `"flow_type":"main-flow"`,
		`"priority":"HIGH"`, `"priority":"critical"`,
		`"risk":"HIGH"`, `"risk":"cao"`,
		`"status":"DRAFT"`, `"status":"pending_review"`,
		`"confidence":0.91`, `"confidence":91`,
	).Replace(validRequirementResponse)
	result, err := ParseResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	item := result.Requirements[0]
	if item.RequirementType != TypeUseCase || item.FlowType != "MAIN" ||
		item.Priority != "HIGH" || item.Risk != "HIGH" || item.Status != StatusDraft ||
		item.Confidence != 0.91 {
		t.Fatalf("normalized proposal=%+v", item)
	}
}

func TestParseResponseReportsInvalidProviderFieldPrecisely(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{strings.Replace(validRequirementResponse, `"flow_type":"MAIN"`, `"flow_type":"UNKNOWN_FLOW"`, 1), `invalid flow_type "UNKNOWN_FLOW"`},
		{strings.Replace(validRequirementResponse, `"confidence":0.91`, `"confidence":101`, 1), "invalid confidence"},
		{strings.Replace(validRequirementResponse, `"status":"DRAFT"`, `"status":"APPROVED"`, 1), `invalid status "APPROVED"`},
	}
	for _, test := range tests {
		_, err := ParseResponse(test.input)
		if !errors.Is(err, ErrInvalidProviderOutput) || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("error=%v, want ErrInvalidProviderOutput containing %q", err, test.want)
		}
	}
}
