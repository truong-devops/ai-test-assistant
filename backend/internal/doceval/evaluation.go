package doceval

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Dataset struct {
	Name         string                 `json:"name"`
	Requirements []ExpectedRequirement  `json:"requirements"`
	Extracted    []ExtractedRequirement `json:"extracted_requirements"`
	TestCases    []TestCaseObservation  `json:"test_cases"`
}
type ExpectedRequirement struct {
	Key      string `json:"key"`
	FlowType string `json:"flow_type"`
}
type ExtractedRequirement struct {
	Key             string `json:"key"`
	FlowType        string `json:"flow_type"`
	Supported       bool   `json:"supported"`
	CitationCorrect bool   `json:"citation_correct"`
}
type TestCaseObservation struct {
	Key                 string  `json:"key"`
	Signature           string  `json:"signature"`
	Variant             string  `json:"variant"`
	HumanAccepted       bool    `json:"human_accepted"`
	EditDistance        int     `json:"edit_distance"`
	ReviewSeconds       float64 `json:"review_seconds"`
	AutomationAttempted bool    `json:"automation_attempted"`
	AutomationCompiled  bool    `json:"automation_compiled"`
	AutomationPassed    bool    `json:"automation_passed"`
	SeededDefect        bool    `json:"seeded_defect"`
	DefectDetected      bool    `json:"defect_detected"`
}
type Rate struct {
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
	Percent     float64 `json:"percent"`
}
type VariantReport struct {
	Cases                  int     `json:"cases"`
	Accepted               int     `json:"accepted"`
	Defects                int     `json:"seeded_defects"`
	Detected               int     `json:"detected_defects"`
	AcceptancePercent      float64 `json:"acceptance_percent"`
	DefectDetectionPercent float64 `json:"defect_detection_percent"`
}
type Report struct {
	Dataset                 string                   `json:"dataset"`
	RequirementPrecision    Rate                     `json:"requirement_precision"`
	RequirementRecall       Rate                     `json:"requirement_recall"`
	FlowCoverageRecall      map[string]Rate          `json:"flow_coverage_recall"`
	UnsupportedClaimRate    Rate                     `json:"unsupported_claim_rate"`
	CitationCorrectness     Rate                     `json:"citation_correctness"`
	DuplicateTestCaseRate   Rate                     `json:"duplicate_test_case_rate"`
	HumanAcceptance         Rate                     `json:"human_acceptance"`
	MeanEditDistance        float64                  `json:"mean_edit_distance"`
	MeanReviewSeconds       float64                  `json:"mean_review_seconds"`
	AutomationCompileRate   Rate                     `json:"automation_compile_rate"`
	AutomationExecutionRate Rate                     `json:"automation_execution_rate"`
	ProductFailureDetection Rate                     `json:"product_failure_detection"`
	Variants                map[string]VariantReport `json:"variants"`
}

func Parse(content []byte) (Dataset, error) {
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	var dataset Dataset
	if err := decoder.Decode(&dataset); err != nil {
		return Dataset{}, err
	}
	if strings.TrimSpace(dataset.Name) == "" || len(dataset.Requirements) == 0 {
		return Dataset{}, errors.New("dataset name and golden requirements are required")
	}
	return dataset, nil
}

func Evaluate(dataset Dataset) (Report, error) {
	if strings.TrimSpace(dataset.Name) == "" || len(dataset.Requirements) == 0 {
		return Report{}, errors.New("dataset name and golden requirements are required")
	}
	expected := map[string]ExpectedRequirement{}
	flowExpected := map[string]int{}
	for _, item := range dataset.Requirements {
		key := normalize(item.Key)
		if key == "" {
			return Report{}, fmt.Errorf("golden requirement key is empty")
		}
		if _, duplicate := expected[key]; duplicate {
			return Report{}, fmt.Errorf("duplicate golden requirement %s", item.Key)
		}
		item.FlowType = flow(item.FlowType)
		expected[key] = item
		flowExpected[item.FlowType]++
	}
	matched, supported, citations := map[string]bool{}, 0, 0
	flowMatched := map[string]int{}
	for _, item := range dataset.Extracted {
		key := normalize(item.Key)
		golden, ok := expected[key]
		if ok && !matched[key] {
			matched[key] = true
			if flow(item.FlowType) == golden.FlowType {
				flowMatched[golden.FlowType]++
			}
		}
		if item.Supported {
			supported++
		}
		if item.CitationCorrect {
			citations++
		}
	}
	report := Report{Dataset: dataset.Name,
		RequirementPrecision: rate(len(matched), len(dataset.Extracted)),
		RequirementRecall:    rate(len(matched), len(expected)),
		UnsupportedClaimRate: rate(len(dataset.Extracted)-supported, len(dataset.Extracted)),
		CitationCorrectness:  rate(citations, len(dataset.Extracted)),
		FlowCoverageRecall:   map[string]Rate{}, Variants: map[string]VariantReport{}}
	for _, name := range []string{"MAIN", "ALTERNATE", "EXCEPTION", "NONE"} {
		report.FlowCoverageRecall[name] = rate(flowMatched[name], flowExpected[name])
	}
	unique, accepted, edits, reviewSeconds := map[string]bool{}, 0, 0, 0.0
	automation, compiled, passed, defects, detected := 0, 0, 0, 0, 0
	for _, item := range dataset.TestCases {
		variantName := strings.ToUpper(strings.TrimSpace(item.Variant))
		if variantName == "" {
			variantName = "UNSPECIFIED"
		}
		// The same business scenario is expected to appear in multiple experiment
		// variants. It is only a duplicate when repeated within one variant.
		unique[variantName+"\x00"+normalize(item.Signature)] = true
		if item.HumanAccepted {
			accepted++
		}
		edits += max(0, item.EditDistance)
		if item.ReviewSeconds > 0 {
			reviewSeconds += item.ReviewSeconds
		}
		if item.AutomationAttempted {
			automation++
			if item.AutomationCompiled {
				compiled++
			}
			if item.AutomationPassed {
				passed++
			}
		}
		if item.SeededDefect {
			defects++
			if item.DefectDetected {
				detected++
			}
		}
		variant := report.Variants[variantName]
		variant.Cases++
		if item.HumanAccepted {
			variant.Accepted++
		}
		if item.SeededDefect {
			variant.Defects++
			if item.DefectDetected {
				variant.Detected++
			}
		}
		report.Variants[variantName] = variant
	}
	for name, variant := range report.Variants {
		variant.AcceptancePercent = percent(variant.Accepted, variant.Cases)
		variant.DefectDetectionPercent = percent(variant.Detected, variant.Defects)
		report.Variants[name] = variant
	}
	duplicates := len(dataset.TestCases) - len(unique)
	if duplicates < 0 {
		duplicates = 0
	}
	report.DuplicateTestCaseRate = rate(duplicates, len(dataset.TestCases))
	report.HumanAcceptance = rate(accepted, len(dataset.TestCases))
	if len(dataset.TestCases) > 0 {
		report.MeanEditDistance = float64(edits) / float64(len(dataset.TestCases))
		report.MeanReviewSeconds = reviewSeconds / float64(len(dataset.TestCases))
	}
	report.AutomationCompileRate = rate(compiled, automation)
	report.AutomationExecutionRate = rate(passed, automation)
	report.ProductFailureDetection = rate(detected, defects)
	return report, nil
}

func rate(numerator, denominator int) Rate {
	return Rate{Numerator: numerator, Denominator: denominator, Percent: percent(numerator, denominator)}
}
func percent(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) * 100 / float64(denominator)
}
func normalize(value string) string { return strings.ToUpper(strings.Join(strings.Fields(value), " ")) }
func flow(value string) string {
	value = normalize(value)
	switch value {
	case "MAIN", "ALTERNATE", "EXCEPTION":
		return value
	}
	return "NONE"
}

func SortedVariants(report Report) []string {
	result := make([]string, 0, len(report.Variants))
	for key := range report.Variants {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
