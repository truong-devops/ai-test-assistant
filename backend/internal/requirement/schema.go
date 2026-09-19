package requirement

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

var ErrInvalidProviderOutput = errors.New("invalid requirement provider output")

type ProposedResponse struct {
	Requirements []Proposal `json:"requirements"`
}

func ResponseSchema() map[string]any {
	stringProperty := func() map[string]any { return map[string]any{"type": "string"} }
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"requirements"},
		"properties": map[string]any{
			"requirements": map[string]any{
				"type": "array", "maxItems": 20,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"identifier", "title", "statement", "requirement_type",
						"flow_type", "actor", "precondition", "postcondition", "priority", "risk",
						"status", "confidence", "assumptions", "steps"},
					"properties": map[string]any{
						"identifier": stringProperty(), "title": stringProperty(), "statement": stringProperty(),
						"requirement_type": map[string]any{"type": "string", "enum": []string{
							TypeFunctional, TypeNonFunctional, TypeBusinessRule, TypeAcceptanceCriterion,
							TypeUseCase, TypeRegression,
						}},
						"flow_type": map[string]any{"type": "string", "enum": []string{
							"NONE", "MAIN", "ALTERNATE", "EXCEPTION",
						}},
						"actor": stringProperty(), "precondition": stringProperty(),
						"postcondition": stringProperty(),
						"priority":      map[string]any{"type": "string", "enum": []string{"LOW", "MEDIUM", "HIGH"}},
						"risk":          map[string]any{"type": "string", "enum": []string{"LOW", "MEDIUM", "HIGH"}},
						"status":        map[string]any{"type": "string", "enum": []string{StatusDraft, StatusTBD}},
						"confidence":    map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"assumptions":   map[string]any{"type": "array", "maxItems": 20, "items": stringProperty()},
						"steps": map[string]any{"type": "array", "maxItems": 100, "items": map[string]any{
							"type": "object", "additionalProperties": false,
							"required":   []string{"action", "expected_result"},
							"properties": map[string]any{"action": stringProperty(), "expected_result": stringProperty()},
						}},
					},
				},
			},
		},
	}
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
	if len(result.Requirements) > 20 {
		return ProposedResponse{}, fmt.Errorf("%w: too many requirements for one semantic unit", ErrInvalidProviderOutput)
	}
	for index := range result.Requirements {
		if err := normalizeProposal(&result.Requirements[index]); err != nil {
			return ProposedResponse{}, fmt.Errorf("%w: requirement %d: %v", ErrInvalidProviderOutput, index+1, err)
		}
	}
	return result, nil
}

func normalizeProposal(item *Proposal) error {
	item.Identifier = strings.ToUpper(strings.TrimSpace(item.Identifier))
	item.Title = strings.TrimSpace(item.Title)
	item.Statement = strings.TrimSpace(item.Statement)
	item.RequirementType = normalizeRequirementType(item.RequirementType)
	item.FlowType = normalizeFlowType(item.FlowType)
	item.Actor = strings.TrimSpace(item.Actor)
	item.Precondition = strings.TrimSpace(item.Precondition)
	item.Postcondition = strings.TrimSpace(item.Postcondition)
	item.Priority = normalizeLevel(item.Priority)
	item.Risk = normalizeLevel(item.Risk)
	item.Status = normalizeDraftStatus(item.Status)
	if item.Confidence > 1 && item.Confidence <= 100 {
		// Prompt-only JSON fallbacks occasionally express confidence as a
		// percentage. It is equivalent information, so normalize it rather than
		// failing a long-running extraction job after a valid provider call.
		item.Confidence /= 100
	}
	if item.Title == "" || item.Statement == "" || utf8.RuneCountInString(item.Title) > 300 ||
		utf8.RuneCountInString(item.Statement) > 12000 || strings.ContainsRune(item.Statement, '\x00') {
		return fmt.Errorf("title or statement is empty, oversized, or invalid")
	}
	if !validRequirementType(item.RequirementType) {
		return fmt.Errorf("invalid requirement_type %q", item.RequirementType)
	}
	if !validFlowType(item.FlowType) {
		return fmt.Errorf("invalid flow_type %q", item.FlowType)
	}
	if !validLevel(item.Priority) {
		return fmt.Errorf("invalid priority %q", item.Priority)
	}
	if !validLevel(item.Risk) {
		return fmt.Errorf("invalid risk %q", item.Risk)
	}
	if item.Status != StatusDraft && item.Status != StatusTBD {
		return fmt.Errorf("invalid status %q: provider cannot approve or reject requirements", item.Status)
	}
	if math.IsNaN(item.Confidence) || math.IsInf(item.Confidence, 0) ||
		item.Confidence < 0 || item.Confidence > 1 {
		return fmt.Errorf("invalid confidence %v: expected a decimal from 0 to 1 or a percentage from 0 to 100", item.Confidence)
	}
	for index := range item.Steps {
		item.Steps[index].Action = strings.TrimSpace(item.Steps[index].Action)
		item.Steps[index].ExpectedResult = strings.TrimSpace(item.Steps[index].ExpectedResult)
		if item.Steps[index].Action == "" {
			return fmt.Errorf("flow step action is required")
		}
	}
	return nil
}

func enumKey(value string) string {
	value = strings.ToUpper(foldVietnameseText(strings.TrimSpace(value)))
	replacer := strings.NewReplacer("-", "_", " ", "_", "/", "_", ".", "_")
	value = replacer.Replace(value)
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func normalizeRequirementType(value string) string {
	switch enumKey(value) {
	case "FUNCTIONAL", "FUNCTIONAL_REQUIREMENT", "FUNCTIONAL_REQUIREMENTS", "FR":
		return TypeFunctional
	case "NON_FUNCTIONAL", "NONFUNCTIONAL", "NON_FUNCTIONAL_REQUIREMENT",
		"NON_FUNCTIONAL_REQUIREMENTS", "NFR":
		return TypeNonFunctional
	case "BUSINESS_RULE", "BUSINESS_RULES", "BUSINESS_REQUIREMENT":
		return TypeBusinessRule
	case "ACCEPTANCE_CRITERION", "ACCEPTANCE_CRITERIA", "ACCEPTANCE":
		return TypeAcceptanceCriterion
	case "USE_CASE", "USECASE", "USER_STORY", "SCENARIO":
		return TypeUseCase
	case "REGRESSION", "REGRESSION_REQUIREMENT":
		return TypeRegression
	default:
		return enumKey(value)
	}
}

func normalizeFlowType(value string) string {
	switch enumKey(value) {
	case "", "NONE", "NA", "N_A", "NOT_APPLICABLE":
		return "NONE"
	case "MAIN", "MAIN_FLOW", "PRIMARY", "PRIMARY_FLOW", "HAPPY_PATH":
		return "MAIN"
	case "ALTERNATE", "ALTERNATIVE", "ALTERNATE_FLOW", "ALTERNATIVE_FLOW":
		return "ALTERNATE"
	case "EXCEPTION", "EXCEPTION_FLOW", "ERROR_FLOW", "FAILURE_FLOW":
		return "EXCEPTION"
	default:
		return enumKey(value)
	}
}

func normalizeLevel(value string) string {
	switch enumKey(value) {
	case "LOW", "MINOR", "THAP":
		return "LOW"
	case "", "MEDIUM", "MODERATE", "NORMAL", "TRUNG_BINH":
		return "MEDIUM"
	case "HIGH", "CRITICAL", "SEVERE", "CAO":
		return "HIGH"
	default:
		return enumKey(value)
	}
}

func normalizeDraftStatus(value string) string {
	switch enumKey(value) {
	case "", "DRAFT", "PROPOSED", "PENDING", "PENDING_REVIEW", "NEEDS_REVIEW":
		return StatusDraft
	case "TBD", "UNCLEAR", "NEEDS_CLARIFICATION", "REQUIRES_CLARIFICATION":
		return StatusTBD
	default:
		// In particular, do not turn provider-supplied APPROVED/REJECTED into a
		// draft. Human ownership of the business baseline is an invariant.
		return enumKey(value)
	}
}

func validRequirementType(value string) bool {
	switch value {
	case TypeFunctional, TypeNonFunctional, TypeBusinessRule, TypeAcceptanceCriterion, TypeUseCase, TypeRegression:
		return true
	default:
		return false
	}
}

func validFlowType(value string) bool {
	return value == "NONE" || value == "MAIN" || value == "ALTERNATE" || value == "EXCEPTION"
}

func validLevel(value string) bool { return value == "LOW" || value == "MEDIUM" || value == "HIGH" }
