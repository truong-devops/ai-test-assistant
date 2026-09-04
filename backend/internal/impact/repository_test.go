package impact

import "testing"

func TestValidateResult(t *testing.T) {
	valid := Result{SourceSHA: "head", Mode: ModeSSA, Algorithm: "cha-v1",
		MaxDepth: 3, MaxNodes: 10, PackageCount: 1,
		Nodes: []Node{{Key: "pkg|Load", SymbolName: "Load", Score: 1,
			ReasonCodes: []string{ReasonDirectChange}}}}
	if err := validateResult(valid); err != nil {
		t.Fatal(err)
	}
	valid.Nodes = append(valid.Nodes, valid.Nodes[0])
	if err := validateResult(valid); err == nil {
		t.Fatal("duplicate node was accepted")
	}
}
