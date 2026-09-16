package testcase

import (
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

func TestGeneratorUsesOnlyApprovedCitedRequirement(t *testing.T) {
	detail := requirement.Detail{Requirement: requirement.Requirement{ID: 1,
		RequirementKey: "BR03", Title: "Giới hạn số lượng", Statement: "Số lượng tối đa là 10",
		RequirementType: requirement.TypeBusinessRule, FlowType: "MAIN", Risk: "HIGH",
		Status: requirement.StatusApproved, Confidence: .9},
		Evidence:  []requirement.Evidence{{ID: 1}},
		FlowSteps: []requirement.FlowStep{{Action: "Nhập số lượng", ExpectedResult: "Không vượt quá 10"}}}
	items := NewGenerator().Generate(detail)
	if len(items) != 2 || items[0].ExpectedResult != detail.Requirement.Statement ||
		items[1].TestType != TypeBoundary {
		t.Fatalf("generated=%+v", items)
	}
	for _, item := range items {
		if len(item.RequirementIDs) != 1 || item.RequirementIDs[0] != detail.Requirement.ID {
			t.Fatalf("proposal lost business source: %+v", item)
		}
	}
}

func TestGeneratorRejectsTBDDraftAndMissingEvidence(t *testing.T) {
	for _, detail := range []requirement.Detail{
		{Requirement: requirement.Requirement{Status: requirement.StatusTBD}},
		{Requirement: requirement.Requirement{Status: requirement.StatusDraft}, Evidence: []requirement.Evidence{{ID: 1}}},
		{Requirement: requirement.Requirement{Status: requirement.StatusApproved}},
	} {
		if items := NewGenerator().Generate(detail); len(items) != 0 {
			t.Fatalf("generated from invalid baseline: %+v", items)
		}
	}
}

func TestGeneratorDoesNotTreatEveryPreconditionAsStateCoverage(t *testing.T) {
	detail := requirement.Detail{Requirement: requirement.Requirement{ID: 2,
		RequirementKey: "YC-02", Title: "Hiển thị địa chỉ", Statement: "Màn hình phải hiển thị địa chỉ nhận hàng",
		RequirementType: requirement.TypeFunctional, FlowType: "NONE", Actor: "Buyer",
		Precondition: "Buyer mở màn hình xác nhận", Risk: "MEDIUM", Status: requirement.StatusApproved,
		Confidence: .82}, Evidence: []requirement.Evidence{{ID: 2}}}
	items := NewGenerator().Generate(detail)
	if len(items) != 1 || items[0].TestType != TypeHappy {
		t.Fatalf("generated=%+v, precondition alone must not invent state coverage", items)
	}
}

func TestDuplicateDetectionSuppressesEquivalentReferenceCases(t *testing.T) {
	kept := []generatedCase{{Case: TestCase{ID: 1}, Proposal: Proposal{Title: "TC-001 - Đặt hàng hợp lệ",
		TestType: TypeHappy, ExpectedResult: "Đơn hàng được tạo với trạng thái chờ thanh toán và hệ thống cấp mã đơn cho Buyer theo dõi"}}}
	exact := kept[0].Proposal
	exact.Title = "TC-001 - Đặt hàng hợp lệ"
	if index, matchType := findDuplicate(kept, exact); index != 0 || matchType != "EXACT" {
		t.Fatalf("exact duplicate index=%d type=%s", index, matchType)
	}
	near := Proposal{Title: "TC-019 - Đặt hàng hợp lệ", TestType: TypeHappy,
		ExpectedResult: "Đơn hàng được tạo với trạng thái chờ thanh toán và hệ thống cấp mã đơn cho Buyer theo dõi"}
	if index, matchType := findDuplicate(kept, near); index != 0 || matchType != "SEMANTIC" {
		t.Fatalf("near duplicate index=%d type=%s", index, matchType)
	}
}
