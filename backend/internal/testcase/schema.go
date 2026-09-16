package testcase

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrInvalidProviderOutput = errors.New("invalid test-case provider output")

type ProposedResponse struct {
	TestCases []Proposal `json:"test_cases"`
}

func ResponseSchema() map[string]any {
	stringProperty := func() map[string]any { return map[string]any{"type": "string"} }
	return map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"test_cases"}, "properties": map[string]any{
			"test_cases": map[string]any{"type": "array", "minItems": 1, "maxItems": 20,
				"items": map[string]any{"type": "object", "additionalProperties": false,
					"required": []string{"title", "test_type", "risk", "actor", "precondition",
						"test_data", "expected_result", "postcondition", "automation_status",
						"confidence", "assumptions", "steps"},
					"properties": map[string]any{
						"title": stringProperty(),
						"test_type": map[string]any{"type": "string", "enum": []string{
							TypeHappy, TypeNegative, TypeBoundary, TypePermission, TypeState,
							TypeIntegration, TypeRegression, TypeNFR}},
						"risk":  map[string]any{"type": "string", "enum": []string{"LOW", "MEDIUM", "HIGH"}},
						"actor": stringProperty(), "precondition": stringProperty(),
						"test_data": stringProperty(), "expected_result": stringProperty(),
						"postcondition":     stringProperty(),
						"automation_status": map[string]any{"type": "string", "enum": []string{"MANUAL", "AUTOMATABLE", "BLOCKED"}},
						"confidence":        map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"assumptions":       map[string]any{"type": "array", "maxItems": 20, "items": stringProperty()},
						"steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 100,
							"items": map[string]any{"type": "object", "additionalProperties": false,
								"required":   []string{"action", "expected_result"},
								"properties": map[string]any{"action": stringProperty(), "expected_result": stringProperty()}}},
					}}}}}
}

func ParseResponse(output string) (ProposedResponse, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(output))
	decoder.DisallowUnknownFields()
	var result ProposedResponse
	if err := decoder.Decode(&result); err != nil {
		return ProposedResponse{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidProviderOutput, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ProposedResponse{}, fmt.Errorf("%w: trailing JSON content", ErrInvalidProviderOutput)
	}
	if len(result.TestCases) < 1 || len(result.TestCases) > 20 {
		return ProposedResponse{}, fmt.Errorf("%w: one requirement must produce 1-20 test cases", ErrInvalidProviderOutput)
	}
	for index := range result.TestCases {
		item := &result.TestCases[index]
		item.Title = strings.TrimSpace(item.Title)
		item.TestType = strings.ToUpper(strings.TrimSpace(item.TestType))
		item.Risk = strings.ToUpper(strings.TrimSpace(item.Risk))
		item.Actor = strings.TrimSpace(item.Actor)
		item.Precondition = strings.TrimSpace(item.Precondition)
		item.TestData = strings.TrimSpace(item.TestData)
		item.ExpectedResult = strings.TrimSpace(item.ExpectedResult)
		item.Postcondition = strings.TrimSpace(item.Postcondition)
		item.AutomationStatus = strings.ToUpper(strings.TrimSpace(item.AutomationStatus))
		if item.Title == "" || item.ExpectedResult == "" || !validTestType(item.TestType) ||
			item.Risk != "LOW" && item.Risk != "MEDIUM" && item.Risk != "HIGH" ||
			item.AutomationStatus != "MANUAL" && item.AutomationStatus != "AUTOMATABLE" && item.AutomationStatus != "BLOCKED" ||
			item.Confidence < 0 || item.Confidence > 1 || len(item.Steps) == 0 {
			return ProposedResponse{}, fmt.Errorf("%w: test case %d has invalid fields", ErrInvalidProviderOutput, index+1)
		}
		for stepIndex := range item.Steps {
			item.Steps[stepIndex].Action = strings.TrimSpace(item.Steps[stepIndex].Action)
			item.Steps[stepIndex].ExpectedResult = strings.TrimSpace(item.Steps[stepIndex].ExpectedResult)
			if item.Steps[stepIndex].Action == "" {
				return ProposedResponse{}, fmt.Errorf("%w: test case %d has an empty step", ErrInvalidProviderOutput, index+1)
			}
		}
	}
	return result, nil
}
