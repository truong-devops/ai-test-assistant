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
