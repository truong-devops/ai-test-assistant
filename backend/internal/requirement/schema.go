package requirement

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	item.RequirementType = strings.ToUpper(strings.TrimSpace(item.RequirementType))
	item.FlowType = strings.ToUpper(strings.TrimSpace(item.FlowType))
	item.Actor = strings.TrimSpace(item.Actor)
	item.Precondition = strings.TrimSpace(item.Precondition)
	item.Postcondition = strings.TrimSpace(item.Postcondition)
	item.Priority = strings.ToUpper(strings.TrimSpace(item.Priority))
	item.Risk = strings.ToUpper(strings.TrimSpace(item.Risk))
	item.Status = strings.ToUpper(strings.TrimSpace(item.Status))
	if item.Title == "" || item.Statement == "" || utf8.RuneCountInString(item.Title) > 300 ||
		utf8.RuneCountInString(item.Statement) > 12000 || strings.ContainsRune(item.Statement, '\x00') {
		return fmt.Errorf("title or statement is empty, oversized, or invalid")
	}
	if !validRequirementType(item.RequirementType) || !validFlowType(item.FlowType) ||
		!validLevel(item.Priority) || !validLevel(item.Risk) ||
		item.Status != StatusDraft && item.Status != StatusTBD || item.Confidence < 0 || item.Confidence > 1 {
		return fmt.Errorf("invalid enum or confidence")
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
