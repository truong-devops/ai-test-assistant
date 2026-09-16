package execution

import "testing"

func TestClassifyResultTaxonomy(t *testing.T) {
	tests := []struct {
		name                 string
		exit                 int
		timeout              bool
		stdout, stderr, path string
		approved             bool
		want                 string
	}{
		{"pass", 0, false, "ok", "", "x_test.go", true, StatusPassed},
		{"timeout", -1, true, "", "", "x_test.go", true, StatusTimedOut},
		{"compile", 1, false, "", "x_test.go:9:2: undefined: missing", "x_test.go", true, StatusAutomationError},
		{"assertion", 1, false, "--- FAIL: TestOrder\n expected created got rejected", "", "x_test.go", true, StatusProductFailed},
		{"infra", 1, false, "", "no required module provides package example.com/dependency", "x_test.go", true, StatusInfraError},
		{"panic in artifact", 1, false, "panic: broken\nx_test.go:12", "", "x_test.go", true, StatusAutomationError},
		{"unapproved", 1, false, "--- FAIL: TestOrder", "", "x_test.go", false, StatusAutomationError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := Classify(test.exit, test.timeout, test.stdout, test.stderr, test.path, test.approved)
			if got != test.want {
				t.Fatalf("Classify()=%s want %s", got, test.want)
			}
		})
	}
}
