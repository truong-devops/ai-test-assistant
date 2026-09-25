package testcase

import "testing"

func TestProposalClassificationRequiresExactContentAndKnownProvenance(t *testing.T) {
	head := proposalHead{Target: GenerationTarget{FamilyID: 1, RevisionID: 2, HeadToken: "pin"}, GenerationHash: "known"}
	for _, tc := range []struct {
		name, provenance, want string
		exact, related         []proposalHead
	}{
		{"new", "known", "NEW_CASE", nil, nil},
		{"unchanged", "known", "UNCHANGED", []proposalHead{head}, nil},
		{"new model", "changed", "NEW_REVISION", []proposalHead{head}, nil},
		{"unknown proof", "", "NEW_REVISION", []proposalHead{head}, nil},
		{"related is not lineage", "known", "AMBIGUOUS_MATCH", nil, []proposalHead{head}},
		{"two exact candidates", "known", "AMBIGUOUS_MATCH", []proposalHead{head, head}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, reason, _ := classifyProposal(tc.provenance, tc.exact, tc.related)
			if kind != tc.want || reason == "" {
				t.Fatalf("%s %s", kind, reason)
			}
		})
	}
	g := GenerationProvenance{Provider: "test", Model: "v1", PromptVersion: "p1", PromptHash: "prompt", RequirementReviewHash: "source"}
	h := generationContextHash(g)
	if h == "" || generationContextHash(GenerationProvenance{}) != "" {
		t.Fatal("unknown generation proof accepted")
	}
	g.WorkflowJobID = 42
	g.ResponseHash = "new raw response formatting"
	g.SourceRevision = 99
	if generationContextHash(g) != h {
		t.Fatal("bookkeeping changed scenario context")
	}
	g.Model = "v2"
	if generationContextHash(g) == h {
		t.Fatal("model change ignored")
	}
}
