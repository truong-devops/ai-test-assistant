package recommendation

import (
	"errors"
	"strings"
	"testing"
)

const validProviderOutput = `{"recommendations":[{"title":"Duplicate email","description":"Cover the new duplicate branch.","priority":"high","rationale":"No existing test covers the branch.","scenario":"Lookup returns an existing user.","expected_behavior":"CreateUser returns ErrEmailExists and does not call Create."}]}`

func TestParseResponseValidatesAndNormalizesOutput(t *testing.T) {
	result, err := ParseResponse(strings.Replace(validProviderOutput, "Duplicate email", "  Duplicate email  ", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Recommendations) != 1 || result.Recommendations[0].Title != "Duplicate email" ||
		result.Recommendations[0].Priority != PriorityHigh {
		t.Fatalf("result = %#v", result)
	}
	schema := ResponseSchema()
	if schema["additionalProperties"] != false || schema["type"] != "object" {
		t.Fatalf("schema = %#v", schema)
	}
}

func TestParseResponseRejectsMalformedProviderOutput(t *testing.T) {
	tests := []string{
		`{`,
		`{"recommendations":[]}`,
		strings.Replace(validProviderOutput, `"high"`, `"urgent"`, 1),
		strings.Replace(validProviderOutput, `}]}`, `}],"extra":true}`, 1),
		validProviderOutput + `{}`,
		strings.Replace(validProviderOutput, `"Duplicate email"`, `""`, 1),
		strings.Replace(validProviderOutput, `}]}`, `},{"title":"duplicate EMAIL","description":"x","priority":"low","rationale":"x","scenario":"x","expected_behavior":"x"}]}`, 1),
	}
	for _, output := range tests {
		if _, err := ParseResponse(output); !errors.Is(err, ErrInvalidProviderOutput) {
			t.Errorf("ParseResponse(%q) error=%v", output, err)
		}
	}
}

func FuzzParseResponse(f *testing.F) {
	f.Add(validProviderOutput)
	f.Add(`{"recommendations":[]}`)
	f.Add(`{`)
	f.Fuzz(func(t *testing.T, output string) {
		if len(output) > 100000 {
			t.Skip()
		}
		_, _ = ParseResponse(output)
	})
}
