package requirement

import "testing"

func TestCompareRequirementsConservativeMapping(t *testing.T) {
	before := []comparisonRequirement{{1, "same", "a"}, {2, "changed", "b"}, {3, "removed", "c"}, {4, "ambiguous", "x"}, {5, "UNKNOWN:5", "missing identifier"}}
	after := []comparisonRequirement{{11, "same", "a"}, {12, "changed", "new b"}, {13, "added", "d"}, {14, "ambiguous", "y"}, {15, "ambiguous", "z"}, {16, "UNKNOWN:16", "missing identifier"}}
	changes := compareRequirements(before, after)
	counts := map[string]int{}
	for _, item := range changes {
		counts[item.Classification]++
		if item.Reason == "" {
			t.Fatal("mapping reason missing")
		}
	}
	if counts["UNCHANGED"] != 1 || counts["CHANGED"] != 1 || counts["REMOVED"] != 1 || counts["ADDED"] != 1 || counts["AMBIGUOUS"] != 3 {
		t.Fatalf("unsafe mapping: %+v", changes)
	}
}
