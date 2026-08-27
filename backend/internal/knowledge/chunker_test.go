package knowledge

import (
	"errors"
	"testing"
)

func TestChunkGoFileCreatesSemanticChunks(t *testing.T) {
	source := []byte(`package user

type Repository interface { Create(User) (User, error) }
type Service struct { repository Repository }
func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) CreateUser(user User) (User, error) { return s.repository.Create(user) }
`)
	chunks, err := ChunkFile("internal/user/service.go", source)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Repository": "interface", "Service": "struct", "NewService": "function", "CreateUser": "method",
	}
	if len(chunks) != len(want) {
		t.Fatalf("chunks = %#v", chunks)
	}
	for _, chunk := range chunks {
		if want[chunk.SymbolName] != chunk.ChunkType {
			t.Errorf("unexpected chunk: %#v", chunk)
		}
		if chunk.PackageName != "user" || chunk.Content == "" || chunk.StartLine <= 0 || chunk.EndLine < chunk.StartLine {
			t.Errorf("invalid chunk metadata: %#v", chunk)
		}
	}
}

func TestChunkTestFileClassifiesTestsMocksAndHelpers(t *testing.T) {
	source := []byte(`package user
type mockRepository struct{}
func (m *mockRepository) Create() {}
func TestCreateUser(t *testing.T) {}
func newFixture() {}
`)
	chunks, err := ChunkFile("internal/user/service_test.go", source)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"mockRepository": "mock", "Create": "mock", "TestCreateUser": "test", "newFixture": "test_helper",
	}
	for _, chunk := range chunks {
		if want[chunk.SymbolName] != chunk.ChunkType {
			t.Errorf("unexpected chunk: %#v", chunk)
		}
	}
}

func TestChunkMarkdownSplitsHeadings(t *testing.T) {
	chunks, err := ChunkFile("docs/business-rules.md", []byte("# Rules\nIntro\n## Email\nMust be unique\n## Order\nLimit quantity\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 || chunks[1].SymbolName != "Email" || chunks[1].StartLine != 3 {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestChunkFileExcludesSensitiveAndGeneratedContent(t *testing.T) {
	tests := []struct {
		path    string
		content string
		wantErr error
	}{
		{".env", "TOKEN=secret", ErrUnsupportedFile},
		{"vendor/a.go", "package a", ErrUnsupportedFile},
		{"private.key", "key", ErrUnsupportedFile},
		{"main.go", "// Code generated tool. DO NOT EDIT.\npackage main", ErrUnsupportedFile},
		{"docs/key.md", "-----BEGIN PRIVATE KEY-----", ErrSensitiveFile},
		{"docs/config.md", `api_key = "this-is-a-secret"`, ErrSensitiveFile},
	}
	for _, test := range tests {
		if _, err := ChunkFile(test.path, []byte(test.content)); !errors.Is(err, test.wantErr) {
			t.Errorf("ChunkFile(%q) error=%v, want %v", test.path, err, test.wantErr)
		}
	}
}

func FuzzChunkFile(f *testing.F) {
	f.Add("main.go", "package main\nfunc main() {}")
	f.Add("README.md", "# Readme\ntext")
	f.Fuzz(func(t *testing.T, filePath, content string) {
		if len(filePath) > 200 || len(content) > 10000 {
			t.Skip()
		}
		_, _ = ChunkFile(filePath, []byte(content))
	})
}
