package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChangeFixtures(t *testing.T) {
	oldFile := parseFixture(t, "change_before.go")
	newFile := parseFixture(t, "change_after.go")
	diff := readFixture(t, "change.diff")
	lines, err := ParseChangedLines(string(diff))
	if err != nil {
		t.Fatal(err)
	}
	changes := MapChangedSymbols(&oldFile, &newFile, lines)
	want := map[string]string{"Keep": "modified", "Delete": "deleted", "Add": "added", "Run": "modified"}
	if len(changes) != len(want) {
		t.Fatalf("changes = %#v", changes)
	}
	for _, change := range changes {
		if want[change.Name] != change.ChangeType {
			t.Errorf("unexpected change: %#v", change)
		}
	}
}

func TestTestOnlyFixture(t *testing.T) {
	newFile := parseFixture(t, "only_test.go")
	oldSource := []byte("package fixture\n\nfunc TestBehavior(t *testing.T) {\n\tprintln(\"old\")\n}\n")
	oldFile, err := ParseGoFile("only_test.go", oldSource)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := ParseChangedLines(string(readFixture(t, "only_test.diff")))
	if err != nil {
		t.Fatal(err)
	}
	changes := MapChangedSymbols(&oldFile, &newFile, lines)
	if len(changes) != 1 || changes[0].Name != "TestBehavior" || changes[0].Kind != "test" ||
		changes[0].ChangeType != "modified" {
		t.Fatalf("changes = %#v", changes)
	}
}

func parseFixture(t *testing.T, name string) ParsedFile {
	t.Helper()
	source := readFixture(t, name)
	result, err := ParseGoFile(name, source)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	result, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
