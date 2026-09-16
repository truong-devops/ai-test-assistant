package doceval

import "testing"

func TestEvaluateSeparatesBusinessAndTechnicalQuality(t *testing.T) {
	report, err := Evaluate(Dataset{Name: "controlled",
		Requirements: []ExpectedRequirement{{Key: "REQ-1", FlowType: "MAIN"}, {Key: "REQ-2", FlowType: "EXCEPTION"}},
		Extracted:    []ExtractedRequirement{{Key: "REQ-1", FlowType: "MAIN", Supported: true, CitationCorrect: true}, {Key: "HALLUCINATED", Supported: false}},
		TestCases: []TestCaseObservation{
			{Key: "TC-1", Signature: "valid order", Variant: "DOC_ONLY_TEST_DESIGN", HumanAccepted: true, AutomationAttempted: true, AutomationCompiled: true, AutomationPassed: true, SeededDefect: true, DefectDetected: true, ReviewSeconds: 10},
			{Key: "TC-2", Signature: "valid order", Variant: "CODE_FIRST", EditDistance: 4, AutomationAttempted: true, SeededDefect: true, ReviewSeconds: 20},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if report.RequirementPrecision.Percent != 50 || report.RequirementRecall.Percent != 50 ||
		report.FlowCoverageRecall["MAIN"].Percent != 100 || report.FlowCoverageRecall["EXCEPTION"].Percent != 0 ||
		report.UnsupportedClaimRate.Percent != 50 || report.CitationCorrectness.Percent != 50 ||
		report.DuplicateTestCaseRate.Percent != 0 || report.AutomationCompileRate.Percent != 50 ||
		report.ProductFailureDetection.Percent != 50 || report.MeanReviewSeconds != 15 {
		t.Fatalf("report=%+v", report)
	}
	if report.Variants["DOC_ONLY_TEST_DESIGN"].AcceptancePercent != 100 || report.Variants["CODE_FIRST"].AcceptancePercent != 0 {
		t.Fatalf("variants=%+v", report.Variants)
	}
}

func TestEvaluateCountsDuplicatesWithinVariantOnly(t *testing.T) {
	report, err := Evaluate(Dataset{
		Name:         "controlled",
		Requirements: []ExpectedRequirement{{Key: "REQ-1", FlowType: "MAIN"}},
		TestCases: []TestCaseObservation{
			{Key: "TC-1", Signature: "valid order", Variant: "DOC_ONLY_TEST_DESIGN"},
			{Key: "TC-2", Signature: "  VALID   ORDER ", Variant: "DOC_ONLY_TEST_DESIGN"},
			{Key: "TC-3", Signature: "valid order", Variant: "CODE_FIRST"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.DuplicateTestCaseRate.Numerator != 1 || report.DuplicateTestCaseRate.Denominator != 3 {
		t.Fatalf("duplicate rate=%+v", report.DuplicateTestCaseRate)
	}
}
