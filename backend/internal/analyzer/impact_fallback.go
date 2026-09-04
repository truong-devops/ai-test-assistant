package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
)

var modulePattern = regexp.MustCompile(`(?m)^\s*module\s+(\S+)\s*$`)

type fallbackFile struct {
	path        string
	packagePath string
	packageName string
	file        *ast.File
	fileSet     *token.FileSet
}

func (e *ImpactEngine) astGraph(root string) (*graphData, int, error) {
	modulePath := "local"
	if contents, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		if match := modulePattern.FindSubmatch(contents); len(match) == 2 {
			modulePath = string(match[1])
		}
	}
	files := make([]fallbackFile, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("AST fallback parse %q: %w", path, err)
		}
		relative, ok := relativeSourcePath(root, path)
		if !ok {
			return nil
		}
		directory := filepath.ToSlash(filepath.Dir(relative))
		packagePath := modulePath
		if directory != "." {
			packagePath += "/" + directory
		}
		files = append(files, fallbackFile{path: relative, packagePath: packagePath,
			packageName: parsed.Name.Name, file: parsed, fileSet: fileSet})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	graph := newGraphData()
	packagePaths := make(map[string]struct{})
	byPackageName := make(map[string]map[string][]string)
	for _, source := range files {
		packagePaths[source.packagePath] = struct{}{}
		if byPackageName[source.packagePath] == nil {
			byPackageName[source.packagePath] = make(map[string][]string)
		}
		for _, declaration := range source.file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				receiver := ""
				if typed.Recv != nil && len(typed.Recv.List) > 0 {
					receiver = receiverName(typed.Recv.List[0].Type)
				}
				kind := "function"
				if receiver != "" {
					kind = "method"
				}
				isTest := strings.HasSuffix(source.path, "_test.go") && strings.HasPrefix(typed.Name.Name, "Test")
				if isTest {
					kind = "test"
				}
				node := sourceNode(source.packagePath, source.packageName, typed.Name.Name,
					receiver, kind, source.path, source.fileSet.Position(typed.Pos()).Line,
					source.fileSet.Position(typed.End()).Line, isTest)
				graph.addNode(node)
				byPackageName[source.packagePath][typed.Name.Name] = append(byPackageName[source.packagePath][typed.Name.Name], node.Key)
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					kind := "type"
					switch typeSpec.Type.(type) {
					case *ast.InterfaceType:
						kind = "interface"
					case *ast.StructType:
						kind = "struct"
					}
					node := sourceNode(source.packagePath, source.packageName, typeSpec.Name.Name,
						"", kind, source.path, source.fileSet.Position(typeSpec.Pos()).Line,
						source.fileSet.Position(typeSpec.End()).Line, false)
					graph.addNode(node)
					byPackageName[source.packagePath][typeSpec.Name.Name] = append(byPackageName[source.packagePath][typeSpec.Name.Name], node.Key)
				}
			}
		}
	}
	for _, source := range files {
		imports := make(map[string]string)
		for _, imported := range source.file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			alias := filepath.Base(path)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			imports[alias] = path
		}
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			owner := fallbackOwner(graph.nodes, source, function)
			if owner == "" {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.CallExpr:
					switch called := typed.Fun.(type) {
					case *ast.Ident:
						for _, target := range byPackageName[source.packagePath][called.Name] {
							graph.addEdge(owner, target, impact.RelationCalls)
						}
					case *ast.SelectorExpr:
						identifier, ok := called.X.(*ast.Ident)
						if ok && imports[identifier.Name] != "" {
							for _, target := range byPackageName[imports[identifier.Name]][called.Sel.Name] {
								graph.addEdge(owner, target, impact.RelationCalls)
							}
						} else {
							for _, target := range byPackageName[source.packagePath][called.Sel.Name] {
								graph.addEdge(owner, target, impact.RelationCalls)
							}
						}
					}
				case *ast.Ident:
					for _, target := range byPackageName[source.packagePath][typed.Name] {
						if graph.nodes[target].SymbolKind == "struct" || graph.nodes[target].SymbolKind == "interface" || graph.nodes[target].SymbolKind == "type" {
							graph.addEdge(owner, target, impact.RelationUsesType)
						}
					}
				}
				return true
			})
		}
	}
	for _, entries := range byPackageName {
		for _, keys := range entries {
			sort.Strings(keys)
		}
	}
	return graph, len(packagePaths), nil
}

func fallbackOwner(nodes map[string]impact.Node, source fallbackFile, function *ast.FuncDecl) string {
	receiver := ""
	if function.Recv != nil && len(function.Recv.List) > 0 {
		receiver = receiverName(function.Recv.List[0].Type)
	}
	for key, node := range nodes {
		if node.PackagePath == source.packagePath && node.FilePath == source.path &&
			node.SymbolName == function.Name.Name && node.ReceiverName == receiver {
			return key
		}
	}
	return ""
}
