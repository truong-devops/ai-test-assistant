package analyzer

import (
	"reflect"
	"testing"
)

func TestParseGoFileDiscoversTopLevelSymbols(t *testing.T) {
	source := []byte(`package service

const DefaultLimit = 20
var ErrMissing = errors.New("missing")
type Identifier string
type User struct { ID Identifier }
type Store interface { Get(Identifier) (User, error) }
func Load() {}
func (u *User) Save() {}
type Box[T any] struct { value T }
func (b *Box[T]) Value() T { return b.value }
`)
	parsed, err := ParseGoFile("service.go", source)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, kind, receiver string }{
		{"DefaultLimit", "constant", ""},
		{"ErrMissing", "variable", ""},
		{"Identifier", "type", ""},
		{"User", "struct", ""},
		{"Store", "interface", ""},
		{"Load", "function", ""},
		{"Save", "method", "User"},
		{"Box", "struct", ""},
		{"Value", "method", "Box"},
	}
	if len(parsed.Symbols) != len(want) {
		t.Fatalf("got %d symbols, want %d: %#v", len(parsed.Symbols), len(want), parsed.Symbols)
	}
	for index, expected := range want {
		got := parsed.Symbols[index]
		if got.Name != expected.name || got.Kind != expected.kind || got.Receiver != expected.receiver {
			t.Errorf("symbol %d = %#v, want name=%q kind=%q receiver=%q", index, got, expected.name, expected.kind, expected.receiver)
		}
		if got.PackageName != "service" || got.StartLine <= 0 || got.EndLine < got.StartLine {
			t.Errorf("invalid symbol metadata: %#v", got)
		}
	}
}

func TestParseGoFileClassifiesTestFunctions(t *testing.T) {
	parsed, err := ParseGoFile("service_test.go", []byte("package service\nfunc TestLoad(t *testing.T) {}\nfunc helper() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Symbols[0].Kind != "test" || parsed.Symbols[1].Kind != "function" {
		t.Fatalf("unexpected symbols: %#v", parsed.Symbols)
	}
}

func TestParseChangedLines(t *testing.T) {
	diff := "--- a/service.go\n+++ b/service.go\n@@ -2,4 +2,5 @@ package service\n keep\n-old\n+new\n+added\n keep\n@@ -20 +21 @@ func other() {\n-old2\n+new2\n"
	lines, err := ParseChangedLines(diff)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines.Old, lineSet(3, 20)) {
		t.Fatalf("old lines = %#v", lines.Old)
	}
	if !reflect.DeepEqual(lines.New, lineSet(3, 4, 21)) {
		t.Fatalf("new lines = %#v", lines.New)
	}
}

func TestParseChangedLinesRejectsMalformedHunk(t *testing.T) {
	if _, err := ParseChangedLines("@@ malformed @@\n+line"); err == nil {
		t.Fatal("expected malformed hunk error")
	}
}

func TestMapChangedSymbolsHandlesAddedModifiedAndDeleted(t *testing.T) {
	oldFile := &ParsedFile{Symbols: []Symbol{
		{Name: "Keep", Kind: "function", StartLine: 2, EndLine: 5},
		{Name: "Remove", Kind: "function", StartLine: 7, EndLine: 9},
	}}
	newFile := &ParsedFile{Symbols: []Symbol{
		{Name: "Keep", Kind: "function", StartLine: 2, EndLine: 6},
		{Name: "Add", Kind: "function", StartLine: 8, EndLine: 10},
	}}
	changes := MapChangedSymbols(oldFile, newFile, ChangedLines{
		Old: lineSet(4, 8), New: lineSet(4, 9),
	})
	want := map[string]string{"Keep": "modified", "Remove": "deleted", "Add": "added"}
	if len(changes) != len(want) {
		t.Fatalf("got %#v", changes)
	}
	for _, change := range changes {
		if want[change.Name] != change.ChangeType {
			t.Errorf("%s type = %s, want %s", change.Name, change.ChangeType, want[change.Name])
		}
		if change.Summary == "" {
			t.Errorf("empty summary for %s", change.Name)
		}
	}
}

func FuzzParseChangedLines(f *testing.F) {
	f.Add("@@ -1 +1 @@\n-old\n+new")
	f.Add("")
	f.Fuzz(func(t *testing.T, diff string) {
		_, _ = ParseChangedLines(diff)
	})
}

func lineSet(lines ...int) map[int]struct{} {
	result := make(map[int]struct{}, len(lines))
	for _, line := range lines {
		result[line] = struct{}{}
	}
	return result
}
