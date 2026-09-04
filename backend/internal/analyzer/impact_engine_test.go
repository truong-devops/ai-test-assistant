package analyzer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/impact"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

type fixtureSCM struct {
	root string
	refs []string
}

func (s *fixtureSCM) GetMergeRequest(context.Context, scm.Repository, int64) (scm.MergeRequest, error) {
	return scm.MergeRequest{}, nil
}
func (s *fixtureSCM) GetMergeRequestDiff(context.Context, scm.Repository, int64) ([]scm.FileDiff, error) {
	return nil, nil
}
func (s *fixtureSCM) GetFileRaw(_ context.Context, _ scm.Repository, filePath, ref string) ([]byte, error) {
	s.refs = append(s.refs, ref)
	return os.ReadFile(filepath.Join(s.root, filepath.FromSlash(filePath)))
}
func (s *fixtureSCM) ListRepositoryTree(_ context.Context, _ scm.Repository, ref string) ([]scm.RepositoryEntry, error) {
	s.refs = append(s.refs, ref)
	var result []scm.RepositoryEntry
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		result = append(result, scm.RepositoryEntry{Type: "blob", Path: filepath.ToSlash(relative)})
		return nil
	})
	return result, err
}

type impactCorpus struct {
	Cases []struct {
		Name            string   `json:"name"`
		FilePath        string   `json:"file_path"`
		PackageName     string   `json:"package_name"`
		SymbolName      string   `json:"symbol_name"`
		SymbolKind      string   `json:"symbol_kind"`
		ExpectedSymbols []string `json:"expected_symbols"`
	} `json:"cases"`
}

func TestImpactEngineLabeledCorpus(t *testing.T) {
	engine, err := NewImpactEngine(DefaultImpactOptions())
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join("testdata", "impact_labels.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus impactCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatal(err)
	}
	truePositive, predicted, expected := 0, 0, 0
	for _, test := range corpus.Cases {
		t.Run(test.Name, func(t *testing.T) {
			result, err := engine.AnalyzeDirectory(context.Background(),
				filepath.Join("testdata", "impact_repo"), "fixture-sha", []impact.DirectSymbol{{
					FilePath: test.FilePath, Symbol: job.ChangedSymbol{SymbolName: test.SymbolName,
						SymbolKind: test.SymbolKind, PackageName: test.PackageName, StartLine: 1, EndLine: 20},
				}})
			if err != nil {
				t.Fatal(err)
			}
			if result.Mode != impact.ModeSSA || result.Algorithm != ImpactAlgorithm || len(result.Nodes) == 0 {
				t.Fatalf("unexpected result metadata: %#v", result)
			}
			actual := make(map[string]bool)
			for _, node := range result.Nodes {
				if !node.DirectChange {
					actual[node.SymbolName] = true
				}
			}
			t.Logf("predicted impacted symbols: %v", actual)
			for _, symbol := range test.ExpectedSymbols {
				if !actual[symbol] {
					t.Errorf("missing expected impacted symbol %q; nodes=%#v", symbol, result.Nodes)
				}
				if actual[symbol] {
					truePositive++
				}
			}
			for _, edge := range result.Edges {
				if edge.ReasonCode == "" || edge.Relation == "" || edge.Depth < 1 || edge.Depth > result.MaxDepth {
					t.Errorf("unexplained or unbounded edge: %#v", edge)
				}
			}
			if test.Name == "cross-package-callers-and-tests" && !nodeHasReason(result.Nodes, "TestLoad", impact.ReasonExistingTest) {
				t.Error("TestLoad is not linked with EXISTING_TEST evidence")
			}
			if test.Name == "interface-implementation" && !nodeHasReason(result.Nodes, "Store", impact.ReasonInterfaceImplementation) {
				t.Error("Store is not linked as an interface implementation")
			}
			if test.Name == "generic-type-usage" && !nodeHasReason(result.Nodes, "Wrap", impact.ReasonTypeUsage) {
				t.Error("Wrap is not linked through generic type usage")
			}
			predicted += len(actual)
			expected += len(test.ExpectedSymbols)
		})
	}
	precision := float64(truePositive) / float64(predicted)
	recall := float64(truePositive) / float64(expected)
	if precision < .50 || recall < .80 {
		t.Fatalf("corpus precision=%.3f recall=%.3f; want precision>=.50 recall>=.80", precision, recall)
	}
}

func nodeHasReason(nodes []impact.Node, symbol, reason string) bool {
	for _, node := range nodes {
		if node.SymbolName != symbol {
			continue
		}
		for _, code := range node.ReasonCodes {
			if code == reason {
				return true
			}
		}
	}
	return false
}

func TestImpactEngineFallsBackToAST(t *testing.T) {
	engine, err := NewImpactEngine(DefaultImpactOptions())
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.AnalyzeDirectory(context.Background(),
		filepath.Join("testdata", "impact_broken"), "broken-sha", []impact.DirectSymbol{{
			FilePath: "broken.go", Symbol: job.ChangedSymbol{SymbolName: "Changed",
				SymbolKind: "function", PackageName: "broken", StartLine: 3, EndLine: 5},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != impact.ModeASTFallback || result.FallbackReason == "" ||
		len(result.Nodes) != 1 || !result.Nodes[0].DirectChange {
		t.Fatalf("fallback result=%#v", result)
	}
}

func TestImpactEngineRejectsEscapingModuleReplacement(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"),
		[]byte("module example.com/unsafe\n\ngo 1.25\n\nreplace example.com/dependency => ../../host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "main.go"),
		[]byte("package unsafe\nfunc Changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine, err := NewImpactEngine(DefaultImpactOptions())
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.AnalyzeDirectory(context.Background(), directory, "source", []impact.DirectSymbol{{
		FilePath: "main.go", Symbol: job.ChangedSymbol{SymbolName: "Changed", SymbolKind: "function",
			PackageName: "unsafe", StartLine: 2, EndLine: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != impact.ModeASTFallback || !strings.Contains(result.FallbackReason, "outside the impact snapshot") {
		t.Fatalf("result=%#v", result)
	}
}

func TestImpactEngineMaterializesExactSourceSHA(t *testing.T) {
	engine, err := NewImpactEngine(DefaultImpactOptions())
	if err != nil {
		t.Fatal(err)
	}
	source := &fixtureSCM{root: filepath.Join("testdata", "impact_repo")}
	result, err := engine.AnalyzeRepository(context.Background(), source,
		scm.Repository{Provider: scm.ProviderGitHub, ProviderProjectID: 1,
			RepositoryURL: "https://github.com/example/fixture"}, "exact-source-sha",
		[]impact.DirectSymbol{{FilePath: "contract/types.go", Symbol: job.ChangedSymbol{
			SymbolName: "Normalize", SymbolKind: "function", PackageName: "contract", StartLine: 13, EndLine: 15}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceSHA != "exact-source-sha" || result.Mode != impact.ModeSSA || len(source.refs) < 2 {
		t.Fatalf("result=%#v refs=%v", result, source.refs)
	}
	for _, ref := range source.refs {
		if strings.TrimSpace(ref) != "exact-source-sha" {
			t.Fatalf("fetched ref=%q", ref)
		}
	}
}

func TestImpactEngineBoundsTraversal(t *testing.T) {
	options := DefaultImpactOptions()
	options.MaxDepth, options.MaxNodes = 1, 2
	engine, err := NewImpactEngine(options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.AnalyzeDirectory(context.Background(),
		filepath.Join("testdata", "impact_repo"), "fixture-sha", []impact.DirectSymbol{{
			FilePath: "contract/types.go", Symbol: job.ChangedSymbol{SymbolName: "Normalize",
				SymbolKind: "function", PackageName: "contract", StartLine: 13, EndLine: 15},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) > 2 {
		t.Fatalf("nodes=%d want <=2", len(result.Nodes))
	}
	for _, node := range result.Nodes {
		if node.Depth > 1 {
			t.Fatalf("node depth=%d", node.Depth)
		}
	}
}

func BenchmarkImpactEngine(b *testing.B) {
	engine, err := NewImpactEngine(DefaultImpactOptions())
	if err != nil {
		b.Fatal(err)
	}
	direct := []impact.DirectSymbol{{FilePath: "contract/types.go", Symbol: job.ChangedSymbol{
		SymbolName: "Reader", SymbolKind: "interface", PackageName: "contract", StartLine: 5, EndLine: 7}}}
	for b.Loop() {
		if _, err := engine.AnalyzeDirectory(context.Background(),
			filepath.Join("testdata", "impact_repo"), "fixture-sha", direct); err != nil {
			b.Fatal(err)
		}
	}
}
