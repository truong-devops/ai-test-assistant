package generation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ErrInvalidProviderOutput = errors.New("invalid generated test provider output")

var (
	testFileNamePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_]*_test\.go$`)
	testFunctionPattern   = regexp.MustCompile(`^Test[A-Z0-9_][A-Za-z0-9_]*$`)
	buildDirectivePattern = regexp.MustCompile(`(?m)^//\s*(?:go:build|\+build)\b`)
)

func ResponseSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"target_file", "test_names", "code"},
		"properties": map[string]any{
			"target_file": map[string]any{"type": "string"},
			"test_names": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 50,
				"items": map[string]any{"type": "string"},
			},
			"code": map[string]any{"type": "string"},
		},
	}
}

func ParseResponse(output, changedFilePath, packageName string) (ProposedGeneration, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(output))
	decoder.DisallowUnknownFields()
	var result ProposedGeneration
	if err := decoder.Decode(&result); err != nil {
		return ProposedGeneration{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidProviderOutput, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ProposedGeneration{}, fmt.Errorf("%w: trailing JSON content", ErrInvalidProviderOutput)
	}
	result.TargetFile = strings.TrimSpace(result.TargetFile)
	result.Code = strings.TrimSpace(result.Code)
	if result.Code != "" {
		result.Code += "\n"
	}
	if err := validateTargetPath(result.TargetFile, changedFilePath); err != nil {
		return ProposedGeneration{}, err
	}
	if err := validateGeneratedCode(&result, strings.TrimSpace(packageName)); err != nil {
		return ProposedGeneration{}, err
	}
	return result, nil
}

func validateTargetPath(targetFile, changedFilePath string) error {
	if targetFile == "" || len(targetFile) > 512 || !utf8.ValidString(targetFile) ||
		strings.ContainsRune(targetFile, '\x00') || strings.Contains(targetFile, `\`) ||
		path.IsAbs(targetFile) || path.Clean(targetFile) != targetFile {
		return fmt.Errorf("%w: invalid target file path", ErrInvalidProviderOutput)
	}
	for _, segment := range strings.Split(targetFile, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("%w: invalid target file path", ErrInvalidProviderOutput)
		}
	}
	if !testFileNamePattern.MatchString(path.Base(targetFile)) ||
		path.Dir(targetFile) != path.Dir(changedFilePath) {
		return fmt.Errorf("%w: target must be a _test.go file beside the changed source", ErrInvalidProviderOutput)
	}
	return nil
}

func validateGeneratedCode(result *ProposedGeneration, packageName string) error {
	if result.Code == "" || len(result.Code) > MaxGeneratedCodeBytes || !utf8.ValidString(result.Code) ||
		strings.ContainsRune(result.Code, '\x00') {
		return fmt.Errorf("%w: generated code is empty, oversized, or invalid", ErrInvalidProviderOutput)
	}
	if buildDirectivePattern.MatchString(result.Code) {
		return fmt.Errorf("%w: generated code must not contain build constraints", ErrInvalidProviderOutput)
	}
	file, err := parser.ParseFile(token.NewFileSet(), result.TargetFile, result.Code, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("%w: invalid Go syntax: %v", ErrInvalidProviderOutput, err)
	}
	if packageName == "" || file.Name == nil ||
		file.Name.Name != packageName && file.Name.Name != packageName+"_test" {
		return fmt.Errorf("%w: generated package does not match changed package", ErrInvalidProviderOutput)
	}
	if len(result.TestNames) < 1 || len(result.TestNames) > 50 {
		return fmt.Errorf("%w: test_names must contain between 1 and 50 names", ErrInvalidProviderOutput)
	}
	declared := make(map[string]struct{})
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Test") {
			continue
		}
		if isGoTestFunction(function) {
			declared[function.Name.Name] = struct{}{}
		}
	}
	listed := make(map[string]struct{}, len(result.TestNames))
	for index, name := range result.TestNames {
		name = strings.TrimSpace(name)
		result.TestNames[index] = name
		if !testFunctionPattern.MatchString(name) {
			return fmt.Errorf("%w: invalid Go test name %q", ErrInvalidProviderOutput, name)
		}
		if _, duplicate := listed[name]; duplicate {
			return fmt.Errorf("%w: duplicate test name %q", ErrInvalidProviderOutput, name)
		}
		if _, exists := declared[name]; !exists {
			return fmt.Errorf("%w: listed test %q must have a non-empty *testing.T body and must not skip",
				ErrInvalidProviderOutput, name)
		}
		listed[name] = struct{}{}
	}
	if len(listed) != len(declared) {
		return fmt.Errorf("%w: test_names must list every generated Test function", ErrInvalidProviderOutput)
	}
	return nil
}

func isGoTestFunction(function *ast.FuncDecl) bool {
	if function.Type == nil || function.Type.Params == nil || len(function.Type.Params.List) != 1 ||
		function.Type.Results != nil && len(function.Type.Results.List) != 0 ||
		function.Body == nil || len(function.Body.List) == 0 {
		return false
	}
	pointer, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != "T" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok || identifier.Name != "testing" {
		return false
	}
	parameterNames := make(map[string]struct{})
	for _, name := range function.Type.Params.List[0].Names {
		parameterNames[name.Name] = struct{}{}
	}
	skips := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selection, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selection.Sel == nil {
			return true
		}
		receiver, ok := selection.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, isTestParameter := parameterNames[receiver.Name]; isTestParameter &&
			(selection.Sel.Name == "Skip" || selection.Sel.Name == "Skipf" || selection.Sel.Name == "SkipNow") {
			skips = true
			return false
		}
		return true
	})
	return !skips
}

func CodeHash(code string) string {
	digest := sha256.Sum256([]byte(code))
	return hex.EncodeToString(digest[:])
}
