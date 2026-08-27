package recommendation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

var ErrInvalidProviderOutput = errors.New("invalid recommendation provider output")

func ResponseSchema() map[string]any {
	stringProperty := func() map[string]any { return map[string]any{"type": "string"} }
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"recommendations"},
		"properties": map[string]any{
			"recommendations": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 10,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"title", "description", "priority", "rationale", "scenario", "expected_behavior",
					},
					"properties": map[string]any{
						"title": stringProperty(), "description": stringProperty(),
						"priority": map[string]any{"type": "string", "enum": []string{
							PriorityLow, PriorityMedium, PriorityHigh,
						}},
						"rationale": stringProperty(), "scenario": stringProperty(),
						"expected_behavior": stringProperty(),
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
	if len(result.Recommendations) < 1 || len(result.Recommendations) > 10 {
		return ProposedResponse{}, fmt.Errorf("%w: recommendations must contain between 1 and 10 items", ErrInvalidProviderOutput)
	}
	seenTitles := make(map[string]struct{}, len(result.Recommendations))
	for index := range result.Recommendations {
		item := &result.Recommendations[index]
		item.Title = strings.TrimSpace(item.Title)
		item.Description = strings.TrimSpace(item.Description)
		item.Priority = strings.TrimSpace(item.Priority)
		item.Rationale = strings.TrimSpace(item.Rationale)
		item.Scenario = strings.TrimSpace(item.Scenario)
		item.ExpectedBehavior = strings.TrimSpace(item.ExpectedBehavior)
		fields := []struct {
			name  string
			value string
			max   int
		}{
			{"title", item.Title, 200}, {"description", item.Description, 4000},
			{"rationale", item.Rationale, 4000}, {"scenario", item.Scenario, 4000},
			{"expected_behavior", item.ExpectedBehavior, 4000},
		}
		for _, field := range fields {
			length := utf8.RuneCountInString(field.value)
			if length < 1 || length > field.max || strings.ContainsRune(field.value, '\x00') {
				return ProposedResponse{}, fmt.Errorf("%w: recommendation %d field %s has invalid length or content",
					ErrInvalidProviderOutput, index, field.name)
			}
		}
		if item.Priority != PriorityLow && item.Priority != PriorityMedium && item.Priority != PriorityHigh {
			return ProposedResponse{}, fmt.Errorf("%w: recommendation %d has invalid priority", ErrInvalidProviderOutput, index)
		}
		titleKey := strings.ToLower(item.Title)
		if _, exists := seenTitles[titleKey]; exists {
			return ProposedResponse{}, fmt.Errorf("%w: duplicate recommendation title", ErrInvalidProviderOutput)
		}
		seenTitles[titleKey] = struct{}{}
	}
	return result, nil
}
