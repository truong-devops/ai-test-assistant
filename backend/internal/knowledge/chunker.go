package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"sort"
	"strings"
)

const MaxIndexFileBytes = 1 << 20

var (
	ErrUnsupportedFile = errors.New("unsupported knowledge file")
	ErrSensitiveFile   = errors.New("sensitive knowledge file")
	ErrInvalidGoFile   = errors.New("invalid Go knowledge file")

	markdownHeadingPattern   = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)
	generatedGoPattern       = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)
	sensitiveContentPatterns = []*regexp.Regexp{
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{16,}\b`),
		regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*["'][^"']{8,}["']`),
		regexp.MustCompile(`(?i)\b(postgres|mysql|mongodb)://[^\s:/]+:[^\s@]+@`),
	}
)

func ChunkFile(filePath string, content []byte) ([]DraftChunk, error) {
	if !IsIndexablePath(filePath) {
		return nil, ErrUnsupportedFile
	}
	if len(content) == 0 || len(content) > MaxIndexFileBytes || bytes.IndexByte(content, 0) >= 0 {
		return nil, ErrUnsupportedFile
	}
	if ContainsSensitiveContent(content) {
		return nil, ErrSensitiveFile
	}
	if strings.HasSuffix(filePath, ".go") {
		if generatedGoPattern.Match(content) {
			return nil, ErrUnsupportedFile
		}
		return chunkGoFile(filePath, content)
	}
	return chunkMarkdown(filePath, content), nil
}

func IsIndexablePath(filePath string) bool {
	cleaned := path.Clean(filePath)
	if filePath == "" || cleaned != filePath || strings.HasPrefix(cleaned, "/") || cleaned == "." {
		return false
	}
	segments := strings.Split(strings.ToLower(cleaned), "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || segment == ".git" ||
			segment == "vendor" || segment == "node_modules" ||
			strings.HasPrefix(segment, ".env") {
			return false
		}
	}
	lower := strings.ToLower(cleaned)
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") ||
		strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") {
		return false
	}
	if strings.HasSuffix(cleaned, ".go") {
		return true
	}
	base := strings.ToLower(path.Base(cleaned))
	return strings.HasSuffix(lower, ".md") && (strings.HasPrefix(base, "readme") ||
		len(segments) > 1 && segments[0] == "docs")
}

func ContainsSensitiveContent(content []byte) bool {
	for _, pattern := range sensitiveContentPatterns {
		if pattern.Match(content) {
			return true
		}
	}
	return false
}

func chunkGoFile(filePath string, source []byte) ([]DraftChunk, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filePath, source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %v", ErrInvalidGoFile, filePath, err)
	}
	chunks := make([]DraftChunk, 0, len(file.Decls))
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			receiver := chunkReceiverName(typed)
			kind := classifyFunction(filePath, typed.Name.Name, receiver)
			chunks = append(chunks, goChunk(fileSet, source, filePath, file.Name.Name,
				typed.Name.Name, receiver, kind, typed, typed.Doc))
		case *ast.GenDecl:
			for _, specification := range typed.Specs {
				switch spec := specification.(type) {
				case *ast.TypeSpec:
					kind := classifyType(filePath, spec.Name.Name, spec.Type)
					node := ast.Node(spec)
					if len(typed.Specs) == 1 {
						node = typed
					}
					doc := spec.Doc
					if doc == nil {
						doc = typed.Doc
					}
					chunks = append(chunks, goChunk(fileSet, source, filePath, file.Name.Name,
						spec.Name.Name, "", kind, node, doc))
				case *ast.ValueSpec:
					kind := "variable"
					if typed.Tok == token.CONST {
						kind = "constant"
					}
					node := ast.Node(spec)
					if len(typed.Specs) == 1 {
						node = typed
					}
					for _, name := range spec.Names {
						chunks = append(chunks, goChunk(fileSet, source, filePath, file.Name.Name,
							name.Name, "", kind, node, typed.Doc))
					}
				}
			}
		}
	}
	if len(chunks) == 0 {
		endLine := 1 + bytes.Count(source, []byte("\n"))
		kind := "implementation_file"
		if strings.HasSuffix(filePath, "_test.go") {
			kind = "test_file"
		}
		chunks = append(chunks, DraftChunk{
			ChunkKey: stableChunkKey(filePath, "go", "file"), FilePath: filePath, PackageName: file.Name.Name,
			ChunkType: kind, Content: string(source), StartLine: 1, EndLine: endLine,
			Metadata: map[string]any{"references": []string{}},
		})
	}
	return chunks, nil
}

func goChunk(fileSet *token.FileSet, source []byte, filePath, packageName, symbolName,
	receiver, chunkType string, node ast.Node, doc *ast.CommentGroup,
) DraftChunk {
	start := node.Pos()
	if doc != nil && doc.Pos() < start {
		start = doc.Pos()
	}
	startPosition := fileSet.Position(start)
	endPosition := fileSet.Position(node.End())
	startOffset, endOffset := startPosition.Offset, endPosition.Offset
	if startOffset < 0 {
		startOffset = 0
	}
	if endOffset > len(source) {
		endOffset = len(source)
	}
	metadata := map[string]any{"references": referencedIdentifiers(node)}
	if receiver != "" {
		metadata["receiver"] = receiver
	}
	return DraftChunk{
		ChunkKey: stableChunkKey(filePath, "go", chunkType, receiver, symbolName),
		FilePath: filePath, PackageName: packageName, SymbolName: symbolName,
		ChunkType: chunkType, Content: string(source[startOffset:endOffset]),
		StartLine: startPosition.Line, EndLine: endPosition.Line, Metadata: metadata,
	}
}

func classifyFunction(filePath, name, receiver string) string {
	lowerName := strings.ToLower(name)
	lowerReceiver := strings.ToLower(receiver)
	if strings.HasSuffix(filePath, "_test.go") {
		switch {
		case receiver == "" && strings.HasPrefix(name, "Test"):
			return "test"
		case strings.Contains(lowerName, "mock") || strings.Contains(lowerReceiver, "mock"):
			return "mock"
		default:
			return "test_helper"
		}
	}
	if receiver != "" {
		return "method"
	}
	return "function"
}

func classifyType(filePath, name string, expression ast.Expr) string {
	lowerName := strings.ToLower(name)
	if strings.HasSuffix(filePath, "_test.go") &&
		(strings.Contains(lowerName, "mock") || strings.Contains(lowerName, "fixture")) {
		return "mock"
	}
	switch expression.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "type"
	}
}

func chunkReceiverName(declaration *ast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return ""
	}
	return receiverExpressionName(declaration.Recv.List[0].Type)
}

func receiverExpressionName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverExpressionName(typed.X)
	case *ast.IndexExpr:
		return receiverExpressionName(typed.X)
	case *ast.IndexListExpr:
		return receiverExpressionName(typed.X)
	case *ast.ParenExpr:
		return receiverExpressionName(typed.X)
	default:
		return ""
	}
}

func referencedIdentifiers(node ast.Node) []string {
	seen := make(map[string]struct{})
	ast.Inspect(node, func(current ast.Node) bool {
		identifier, ok := current.(*ast.Ident)
		if ok && identifier.Name != "_" {
			seen[identifier.Name] = struct{}{}
		}
		return true
	})
	result := make([]string, 0, len(seen))
	for identifier := range seen {
		result = append(result, identifier)
	}
	sort.Strings(result)
	return result
}

func chunkMarkdown(filePath string, content []byte) []DraftChunk {
	lines := strings.Split(string(content), "\n")
	starts := []int{0}
	headings := map[int]string{0: path.Base(filePath)}
	for index, line := range lines {
		matches := markdownHeadingPattern.FindStringSubmatch(line)
		if len(matches) == 2 && index != 0 {
			starts = append(starts, index)
			headings[index] = strings.TrimSpace(matches[1])
		} else if len(matches) == 2 {
			headings[0] = strings.TrimSpace(matches[1])
		}
	}
	chunks := make([]DraftChunk, 0, len(starts))
	for ordinal, start := range starts {
		end := len(lines)
		if ordinal+1 < len(starts) {
			end = starts[ordinal+1]
		}
		text := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if text == "" {
			continue
		}
		chunks = append(chunks, DraftChunk{
			ChunkKey: stableChunkKey(filePath, "doc", fmt.Sprintf("%d", ordinal)), FilePath: filePath,
			SymbolName: headings[start], ChunkType: "documentation", Content: text,
			StartLine: start + 1, EndLine: end, Metadata: map[string]any{"heading": headings[start]},
		})
	}
	return chunks
}

func stableChunkKey(parts ...string) string {
	hasher := sha256.New()
	for _, part := range parts {
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(part))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
