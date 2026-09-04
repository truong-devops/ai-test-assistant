package analyzer

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

const (
	ImpactAlgorithm   = "cha-v1+x-tools-v0.49.0"
	FallbackAlgorithm = "ast-v1"
)

type ImpactOptions struct {
	MaxDepth      int
	MaxNodes      int
	MaxFiles      int
	MaxFileBytes  int
	MaxTotalBytes int
	Timeout       time.Duration
}

func DefaultImpactOptions() ImpactOptions {
	return ImpactOptions{MaxDepth: 3, MaxNodes: 250, MaxFiles: 2_000,
		MaxFileBytes: 1 << 20, MaxTotalBytes: 64 << 20, Timeout: 2 * time.Minute}
}

type ImpactEngine struct{ options ImpactOptions }

func NewImpactEngine(options ImpactOptions) (*ImpactEngine, error) {
	if options.MaxDepth < 1 || options.MaxDepth > 20 || options.MaxNodes < 1 ||
		options.MaxNodes > 10_000 || options.MaxFiles < 1 || options.MaxFileBytes < 1 ||
		options.MaxTotalBytes < options.MaxFileBytes || options.Timeout < time.Second ||
		options.Timeout > 30*time.Minute {
		return nil, fmt.Errorf("invalid impact analysis limits")
	}
	return &ImpactEngine{options: options}, nil
}

func (e *ImpactEngine) AnalyzeRepository(ctx context.Context, source scm.Client,
	repository scm.Repository, sourceSHA string, direct []impact.DirectSymbol,
) (impact.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, e.options.Timeout)
	defer cancel()
	workspace, cleanup, err := e.materialize(ctx, source, repository, sourceSHA)
	if err != nil {
		return impact.Result{}, err
	}
	defer cleanup()
	return e.AnalyzeDirectory(ctx, workspace, sourceSHA, direct)
}

func (e *ImpactEngine) AnalyzeDirectory(ctx context.Context, directory, sourceSHA string,
	direct []impact.DirectSymbol,
) (impact.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, e.options.Timeout)
	defer cancel()
	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		return impact.Result{}, fmt.Errorf("resolve impact workspace: %w", err)
	}
	directory = absoluteDirectory
	graph, packageCount, err := e.typedGraph(ctx, directory)
	if err == nil {
		return e.selectImpact(sourceSHA, impact.ModeSSA, ImpactAlgorithm, "",
			packageCount, graph, direct), nil
	}
	fallback, fallbackPackages, fallbackErr := e.astGraph(directory)
	if fallbackErr != nil {
		return impact.Result{}, errors.Join(err, fallbackErr)
	}
	return e.selectImpact(sourceSHA, impact.ModeASTFallback, FallbackAlgorithm,
		truncateReason(err.Error()), fallbackPackages, fallback, direct), nil
}

func (e *ImpactEngine) materialize(ctx context.Context, source scm.Client,
	repository scm.Repository, ref string,
) (string, func(), error) {
	if strings.TrimSpace(ref) == "" {
		return "", nil, fmt.Errorf("impact source SHA is required")
	}
	entries, err := source.ListRepositoryTree(ctx, repository, ref)
	if err != nil {
		return "", nil, fmt.Errorf("list impact source tree at %s: %w", ref, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	directory, err := os.MkdirTemp("", "ai-test-impact-*")
	if err != nil {
		return "", nil, fmt.Errorf("create impact workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	fileCount, totalBytes := 0, 0
	for _, entry := range entries {
		if entry.Type != "blob" || !impactSourcePath(entry.Path) {
			continue
		}
		if !fs.ValidPath(entry.Path) || strings.Contains(entry.Path, `\`) {
			cleanup()
			return "", nil, fmt.Errorf("unsafe impact source path %q", entry.Path)
		}
		fileCount++
		if fileCount > e.options.MaxFiles {
			cleanup()
			return "", nil, fmt.Errorf("impact source exceeds %d files", e.options.MaxFiles)
		}
		contents, err := source.GetFileRaw(ctx, repository, entry.Path, ref)
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("fetch impact source %q: %w", entry.Path, err)
		}
		if len(contents) > e.options.MaxFileBytes || totalBytes+len(contents) > e.options.MaxTotalBytes {
			cleanup()
			return "", nil, fmt.Errorf("impact source size limit exceeded at %q", entry.Path)
		}
		totalBytes += len(contents)
		target := filepath.Join(directory, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("create impact source directory: %w", err)
		}
		if err := os.WriteFile(target, contents, 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("write impact source %q: %w", entry.Path, err)
		}
	}
	if fileCount == 0 {
		cleanup()
		return "", nil, fmt.Errorf("repository has no Go module source at %s", ref)
	}
	return directory, cleanup, nil
}

func impactSourcePath(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(path, ".go") || base == "go.mod" || base == "go.sum" ||
		base == "go.work" || base == "go.work.sum" || path == "vendor/modules.txt"
}

type graphData struct {
	nodes    map[string]impact.Node
	outgoing map[string][]rawEdge
	incoming map[string][]rawEdge
}

type rawEdge struct{ from, to, relation string }

func newGraphData() *graphData {
	return &graphData{nodes: make(map[string]impact.Node), outgoing: make(map[string][]rawEdge),
		incoming: make(map[string][]rawEdge)}
}

func (g *graphData) addNode(node impact.Node) {
	if existing, ok := g.nodes[node.Key]; ok {
		existing.ExistingTest = existing.ExistingTest || node.ExistingTest
		g.nodes[node.Key] = existing
		return
	}
	g.nodes[node.Key] = node
}

func (g *graphData) addEdge(from, to, relation string) {
	if from == "" || to == "" || from == to {
		return
	}
	edge := rawEdge{from: from, to: to, relation: relation}
	g.outgoing[from] = append(g.outgoing[from], edge)
	g.incoming[to] = append(g.incoming[to], edge)
}

func (e *ImpactEngine) typedGraph(ctx context.Context, root string) (*graphData, int, error) {
	if err := validateModulePaths(root); err != nil {
		return nil, 0, err
	}
	cacheDirectory, err := os.MkdirTemp("", "ai-test-impact-cache-*")
	if err != nil {
		return nil, 0, fmt.Errorf("create impact Go cache: %w", err)
	}
	defer os.RemoveAll(cacheDirectory)
	cfg := &packages.Config{Context: ctx, Dir: root, Tests: true, Mode: packages.LoadSyntax,
		Env: impactGoEnv(os.Environ(), root, cacheDirectory)}
	loaded, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, 0, fmt.Errorf("load packages: %w", err)
	}
	if len(loaded) == 0 {
		return nil, 0, fmt.Errorf("load packages: no packages found")
	}
	var loadErrors []string
	for _, pkg := range loaded {
		for _, packageErr := range pkg.Errors {
			loadErrors = append(loadErrors, packageErr.Error())
		}
	}
	if len(loadErrors) > 0 {
		sort.Strings(loadErrors)
		return nil, 0, fmt.Errorf("type-check packages: %s", strings.Join(loadErrors, "; "))
	}
	graph := newGraphData()
	objectKeys := make(map[types.Object]string)
	positionKeys := make(map[string]string)
	typeObjects := make([]typedNode, 0)
	packagePaths := make(map[string]struct{})
	for _, pkg := range loaded {
		if !localPackage(pkg, root) || pkg.TypesInfo == nil {
			continue
		}
		packagePath := normalizedPackagePath(pkg)
		packagePaths[packagePath] = struct{}{}
		for index, file := range pkg.Syntax {
			filename := syntaxFilename(pkg, index, file)
			relative, ok := relativeSourcePath(root, filename)
			if !ok {
				continue
			}
			for _, declaration := range file.Decls {
				switch typed := declaration.(type) {
				case *ast.FuncDecl:
					object := pkg.TypesInfo.Defs[typed.Name]
					receiver := ""
					if function, ok := object.(*types.Func); ok {
						receiver = receiverFromObject(function)
					}
					kind := "function"
					if receiver != "" {
						kind = "method"
					}
					isTest := strings.HasSuffix(relative, "_test.go") && strings.HasPrefix(typed.Name.Name, "Test")
					if isTest {
						kind = "test"
					}
					node := sourceNode(packagePath, pkg.Name, typed.Name.Name, receiver, kind,
						relative, pkg.Fset.Position(typed.Pos()).Line, pkg.Fset.Position(typed.End()).Line, isTest)
					graph.addNode(node)
					if object != nil {
						objectKeys[object] = node.Key
					}
					positionKeys[positionKey(filename, node.StartLine, node.SymbolName)] = node.Key
				case *ast.GenDecl:
					for _, specification := range typed.Specs {
						typeSpec, ok := specification.(*ast.TypeSpec)
						if !ok {
							continue
						}
						object, _ := pkg.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
						kind := "type"
						if object != nil {
							switch object.Type().Underlying().(type) {
							case *types.Interface:
								kind = "interface"
							case *types.Struct:
								kind = "struct"
							}
						}
						node := sourceNode(packagePath, pkg.Name, typeSpec.Name.Name, "", kind,
							relative, pkg.Fset.Position(typeSpec.Pos()).Line, pkg.Fset.Position(typeSpec.End()).Line, false)
						graph.addNode(node)
						if object != nil {
							objectKeys[object] = node.Key
							typeObjects = append(typeObjects, typedNode{key: node.Key, object: object})
						}
					}
				}
			}
		}
	}
	program, _ := ssautil.Packages(loaded, ssa.InstantiateGenerics)
	program.Build()
	callGraph := cha.CallGraph(program)
	for function, callNode := range callGraph.Nodes {
		from := ssaFunctionKey(function, root, positionKeys)
		if from == "" {
			continue
		}
		for _, edge := range callNode.Out {
			to := ssaFunctionKey(edge.Callee.Func, root, positionKeys)
			if to != "" {
				graph.addEdge(from, to, impact.RelationCalls)
			}
		}
	}
	for _, pkg := range loaded {
		if !localPackage(pkg, root) || pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, declaration := range file.Decls {
				owner := declarationOwnerKey(declaration, pkg.TypesInfo, objectKeys)
				if owner == "" {
					continue
				}
				ast.Inspect(declaration, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if !ok {
						return true
					}
					used, ok := pkg.TypesInfo.Uses[identifier].(*types.TypeName)
					if ok {
						if target := objectKeys[used]; target != "" {
							graph.addEdge(owner, target, impact.RelationUsesType)
						}
					}
					return true
				})
			}
		}
	}
	for _, candidate := range typeObjects {
		if _, isInterface := candidate.object.Type().Underlying().(*types.Interface); isInterface {
			continue
		}
		for _, target := range typeObjects {
			contract, ok := target.object.Type().Underlying().(*types.Interface)
			if !ok || contract.NumMethods() == 0 || candidate.key == target.key {
				continue
			}
			if types.Implements(candidate.object.Type(), contract) ||
				types.Implements(types.NewPointer(candidate.object.Type()), contract) {
				graph.addEdge(candidate.key, target.key, impact.RelationImplements)
			}
		}
	}
	return graph, len(packagePaths), nil
}

type typedNode struct {
	key    string
	object *types.TypeName
}

func (e *ImpactEngine) selectImpact(sourceSHA, mode, algorithm, fallback string,
	packageCount int, graph *graphData, direct []impact.DirectSymbol,
) impact.Result {
	selected := make(map[string]impact.Node)
	type queueItem struct {
		key   string
		depth int
	}
	queue := make([]queueItem, 0)
	directSorted := append([]impact.DirectSymbol(nil), direct...)
	sort.Slice(directSorted, func(i, j int) bool {
		if directSorted[i].FilePath != directSorted[j].FilePath {
			return directSorted[i].FilePath < directSorted[j].FilePath
		}
		return directSorted[i].Symbol.SymbolName < directSorted[j].Symbol.SymbolName
	})
	for _, changed := range directSorted {
		matches := matchingNodes(graph.nodes, changed)
		if len(matches) == 0 {
			node := directNode(changed)
			graph.addNode(node)
			matches = []string{node.Key}
		}
		for _, key := range matches {
			if len(selected) >= e.options.MaxNodes {
				break
			}
			node := graph.nodes[key]
			node.DirectChange, node.Depth, node.Score = true, 0, 1
			node.ReasonCodes = addReason(node.ReasonCodes, impact.ReasonDirectChange)
			selected[key] = node
			queue = append(queue, queueItem{key: key})
		}
	}
	edges := make(map[string]impact.Edge)
	visitedDepth := make(map[string]int)
	for _, item := range queue {
		visitedDepth[item.key] = 0
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= e.options.MaxDepth {
			continue
		}
		walk := func(raw rawEdge, neighbor string, incoming bool) {
			reason := reasonForEdge(raw, graph.nodes[neighbor], incoming)
			depth := current.depth + 1
			score := impactScore(reason, depth)
			if existingDepth, ok := visitedDepth[neighbor]; !ok || depth < existingDepth {
				if _, exists := selected[neighbor]; !exists && len(selected) >= e.options.MaxNodes {
					return
				}
				node, exists := graph.nodes[neighbor]
				if !exists {
					return
				}
				node.Depth, node.Score = depth, score
				node.ReasonCodes = addReason(node.ReasonCodes, reason)
				selected[neighbor] = node
				visitedDepth[neighbor] = depth
				queue = append(queue, queueItem{key: neighbor, depth: depth})
			} else if node, exists := selected[neighbor]; exists {
				node.ReasonCodes = addReason(node.ReasonCodes, reason)
				if score > node.Score {
					node.Score = score
				}
				selected[neighbor] = node
			}
			if _, fromOK := selected[raw.from]; fromOK {
				if _, toOK := selected[raw.to]; toOK {
					key := raw.from + "\x00" + raw.to + "\x00" + reason
					edges[key] = impact.Edge{FromKey: raw.from, ToKey: raw.to,
						Relation: raw.relation, ReasonCode: reason, Depth: depth, Score: score}
				}
			}
		}
		for _, raw := range graph.outgoing[current.key] {
			walk(raw, raw.to, false)
		}
		for _, raw := range graph.incoming[current.key] {
			walk(raw, raw.from, true)
		}
	}
	result := impact.Result{SourceSHA: sourceSHA, Mode: mode, Algorithm: algorithm,
		MaxDepth: e.options.MaxDepth, MaxNodes: e.options.MaxNodes,
		PackageCount: packageCount, FallbackReason: fallback}
	for _, node := range selected {
		sort.Strings(node.ReasonCodes)
		result.Nodes = append(result.Nodes, node)
	}
	sort.Slice(result.Nodes, func(i, j int) bool {
		if result.Nodes[i].DirectChange != result.Nodes[j].DirectChange {
			return result.Nodes[i].DirectChange
		}
		if result.Nodes[i].Score != result.Nodes[j].Score {
			return result.Nodes[i].Score > result.Nodes[j].Score
		}
		return result.Nodes[i].Key < result.Nodes[j].Key
	})
	for _, edge := range edges {
		result.Edges = append(result.Edges, edge)
	}
	sort.Slice(result.Edges, func(i, j int) bool {
		if result.Edges[i].Depth != result.Edges[j].Depth {
			return result.Edges[i].Depth < result.Edges[j].Depth
		}
		if result.Edges[i].FromKey != result.Edges[j].FromKey {
			return result.Edges[i].FromKey < result.Edges[j].FromKey
		}
		if result.Edges[i].ToKey != result.Edges[j].ToKey {
			return result.Edges[i].ToKey < result.Edges[j].ToKey
		}
		return result.Edges[i].ReasonCode < result.Edges[j].ReasonCode
	})
	return result
}

func reasonForEdge(edge rawEdge, neighbor impact.Node, incoming bool) string {
	switch edge.relation {
	case impact.RelationImplements:
		return impact.ReasonInterfaceImplementation
	case impact.RelationUsesType:
		return impact.ReasonTypeUsage
	default:
		if neighbor.ExistingTest {
			return impact.ReasonExistingTest
		}
		if incoming {
			return impact.ReasonCaller
		}
		return impact.ReasonCallee
	}
}

func impactScore(reason string, depth int) float64 {
	base := map[string]float64{impact.ReasonCaller: .88, impact.ReasonCallee: .78,
		impact.ReasonInterfaceImplementation: .84, impact.ReasonTypeUsage: .74,
		impact.ReasonExistingTest: .92}[reason]
	score := base - float64(depth-1)*.12
	if score < .1 {
		return .1
	}
	return score
}

func matchingNodes(nodes map[string]impact.Node, direct impact.DirectSymbol) []string {
	result := make([]string, 0)
	wantFile := filepath.ToSlash(direct.FilePath)
	for key, node := range nodes {
		if node.SymbolName != direct.Symbol.SymbolName || node.PackageName != direct.Symbol.PackageName ||
			(node.ReceiverName != "" && direct.Symbol.ReceiverName != "" && node.ReceiverName != direct.Symbol.ReceiverName) {
			continue
		}
		if wantFile != "" && node.FilePath != wantFile {
			continue
		}
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func directNode(value impact.DirectSymbol) impact.Node {
	symbol := value.Symbol
	key := "direct|" + filepath.ToSlash(value.FilePath) + "|" + symbol.ReceiverName + "|" + symbol.SymbolName
	return impact.Node{Key: key, PackageName: symbol.PackageName, SymbolName: symbol.SymbolName,
		ReceiverName: symbol.ReceiverName, SymbolKind: symbol.SymbolKind,
		FilePath: filepath.ToSlash(value.FilePath), StartLine: symbol.StartLine,
		EndLine: symbol.EndLine, DirectChange: true, Score: 1,
		ReasonCodes: []string{impact.ReasonDirectChange}}
}

func sourceNode(packagePath, packageName, name, receiver, kind, file string,
	start, end int, test bool,
) impact.Node {
	key := strings.Join([]string{packagePath, filepath.ToSlash(file), receiver, name, kind}, "|")
	return impact.Node{Key: key, PackagePath: packagePath, PackageName: packageName,
		SymbolName: name, ReceiverName: receiver, SymbolKind: kind,
		FilePath: filepath.ToSlash(file), StartLine: start, EndLine: end, ExistingTest: test}
}

func addReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func receiverFromObject(function *types.Func) string {
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return ""
	}
	typeValue := signature.Recv().Type()
	if pointer, ok := typeValue.(*types.Pointer); ok {
		typeValue = pointer.Elem()
	}
	if named, ok := typeValue.(*types.Named); ok {
		return named.Obj().Name()
	}
	return types.TypeString(typeValue, func(*types.Package) string { return "" })
}

func normalizedPackagePath(pkg *packages.Package) string {
	if pkg.ForTest != "" && pkg.Name != "main" {
		return pkg.ForTest
	}
	return pkg.PkgPath
}

func localPackage(pkg *packages.Package, root string) bool {
	for _, file := range pkg.CompiledGoFiles {
		if _, ok := relativeSourcePath(root, file); ok {
			return true
		}
	}
	return false
}

func relativeSourcePath(root, filename string) (string, bool) {
	relative, err := filepath.Rel(root, filename)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func syntaxFilename(pkg *packages.Package, index int, file *ast.File) string {
	if index < len(pkg.CompiledGoFiles) {
		return pkg.CompiledGoFiles[index]
	}
	return pkg.Fset.Position(file.Pos()).Filename
}

func positionKey(file string, line int, name string) string {
	return filepath.Clean(file) + fmt.Sprintf(":%d:%s", line, name)
}

func ssaFunctionKey(function *ssa.Function, root string, positions map[string]string) string {
	if function == nil || !function.Pos().IsValid() {
		return ""
	}
	position := function.Prog.Fset.Position(function.Pos())
	if _, ok := relativeSourcePath(root, position.Filename); !ok {
		return ""
	}
	return positions[positionKey(position.Filename, position.Line, function.Name())]
}

func declarationOwnerKey(declaration ast.Decl, info *types.Info, keys map[types.Object]string) string {
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		return keys[info.Defs[typed.Name]]
	case *ast.GenDecl:
		for _, specification := range typed.Specs {
			if typeSpec, ok := specification.(*ast.TypeSpec); ok {
				return keys[info.Defs[typeSpec.Name]]
			}
		}
	}
	return ""
}

func impactGoEnv(environment []string, root, cacheDirectory string) []string {
	blocked := map[string]bool{"GOWORK": true, "GOPROXY": true, "GOSUMDB": true,
		"CGO_ENABLED": true, "GOFLAGS": true, "GOMODCACHE": true}
	result := make([]string, 0, len(environment)+5)
	for _, item := range environment {
		name := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			name = item[:index]
		}
		if !blocked[name] {
			result = append(result, item)
		}
	}
	flags := "-mod=mod"
	if _, err := os.Stat(filepath.Join(root, "vendor", "modules.txt")); err == nil {
		flags = "-mod=vendor"
	}
	return append(result, "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=0",
		"GOMODCACHE="+filepath.Join(cacheDirectory, "modules"), "GOFLAGS="+flags)
}

func validateModulePaths(root string) error {
	contents, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read go.mod for impact analysis: %w", err)
	}
	parsed, err := modfile.Parse("go.mod", contents, nil)
	if err != nil {
		return fmt.Errorf("parse go.mod for impact analysis: %w", err)
	}
	for _, replacement := range parsed.Replace {
		if replacement.New.Version != "" {
			continue
		}
		path := replacement.New.Path
		if filepath.IsAbs(path) {
			return fmt.Errorf("local module replacement outside the impact snapshot is disabled: %q", path)
		}
		resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
		if _, ok := relativeSourcePath(root, resolved); !ok {
			return fmt.Errorf("local module replacement outside the impact snapshot is disabled: %q", path)
		}
	}
	return nil
}

func truncateReason(value string) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "�")
	limit := 16_384
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit]
}
