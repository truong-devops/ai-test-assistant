package generation

import (
	"errors"
	"strings"
	"testing"
)

const validGeneratedOutput = `{"target_file":"internal/user/service_generated_test.go","test_names":["TestService_CreateUser_DuplicateEmail"],"code":"package user\n\nimport \"testing\"\n\nfunc TestService_CreateUser_DuplicateEmail(t *testing.T) { t.Helper() }\n"}`

func TestParseResponseValidatesGeneratedGoTest(t *testing.T) {
	result, err := ParseResponse(validGeneratedOutput, "internal/user/service.go", "user")
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetFile != "internal/user/service_generated_test.go" || len(result.TestNames) != 1 ||
		result.TestNames[0] != "TestService_CreateUser_DuplicateEmail" ||
		!strings.HasSuffix(result.Code, "\n") || len(CodeHash(result.Code)) != 64 {
		t.Fatalf("result=%#v", result)
	}
	schema := ResponseSchema()
	if schema["additionalProperties"] != false || schema["type"] != "object" {
		t.Fatalf("schema=%#v", schema)
	}
}

func TestParseResponseRejectsEmptyCode(t *testing.T) {
	output := strings.Replace(validGeneratedOutput,
		`"package user\n\nimport \"testing\"\n\nfunc TestService_CreateUser_DuplicateEmail(t *testing.T) { t.Helper() }\n"`, `""`, 1)
	if _, err := ParseResponse(output, "internal/user/service.go", "user"); !errors.Is(err, ErrInvalidProviderOutput) {
		t.Fatalf("ParseResponse() error=%v", err)
	}
}

func TestParseResponseRejectsInvalidTargetPaths(t *testing.T) {
	paths := []string{
		"../service_generated_test.go", "/tmp/service_test.go", "internal/other/service_test.go",
		"internal/user/service.go", `internal\user\service_test.go`, "internal/user/_service_test.go",
	}
	for _, target := range paths {
		output := strings.Replace(validGeneratedOutput, "internal/user/service_generated_test.go", target, 1)
		if _, err := ParseResponse(output, "internal/user/service.go", "user"); !errors.Is(err, ErrInvalidProviderOutput) {
			t.Errorf("target=%q error=%v", target, err)
		}
	}
}

func TestParseResponseRejectsUnsafeOrInconsistentCode(t *testing.T) {
	tests := []string{
		strings.Replace(validGeneratedOutput, "package user", "package other", 1),
		strings.Replace(validGeneratedOutput, "package user", "//go:build never\npackage user", 1),
		strings.Replace(validGeneratedOutput, "func TestService_CreateUser_DuplicateEmail", "func helper", 1),
		strings.Replace(validGeneratedOutput, "*testing.T", "testing.T", 1),
		strings.Replace(validGeneratedOutput, "{ t.Helper() }", "{}", 1),
		strings.Replace(validGeneratedOutput, "t.Helper()", `t.Skip(\"disabled\")`, 1),
		strings.Replace(validGeneratedOutput, `"TestService_CreateUser_DuplicateEmail"`, `"TestMissing"`, 1),
		strings.Replace(validGeneratedOutput, `}`, `,"extra":true}`, 1),
		validGeneratedOutput + `{}`,
	}
	for _, output := range tests {
		if _, err := ParseResponse(output, "internal/user/service.go", "user"); !errors.Is(err, ErrInvalidProviderOutput) {
			t.Errorf("output=%q error=%v", output, err)
		}
	}
}

func FuzzParseResponse(f *testing.F) {
	f.Add(validGeneratedOutput)
	f.Add(`{`)
	f.Add(`{"target_file":"../x_test.go","test_names":[],"code":""}`)
	f.Fuzz(func(t *testing.T, output string) {
		if len(output) > 100000 {
			t.Skip()
		}
		_, _ = ParseResponse(output, "internal/user/service.go", "user")
	})
}
