package validation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/scm"
)

type repositorySourceStub struct {
	entries []gitlab.RepositoryEntry
	files   map[string][]byte
	listErr error
	getErr  error
}

func (s repositorySourceStub) ListRepositoryTree(context.Context, scm.Repository, string) ([]gitlab.RepositoryEntry, error) {
	return s.entries, s.listErr
}

func (s repositorySourceStub) GetFileRaw(_ context.Context, _ scm.Repository, filePath, _ string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.files[filePath], nil
}

func TestWorkspaceManagerPreparesIsolatedSnapshotAndCleansIt(t *testing.T) {
	source := repositorySourceStub{
		entries: []gitlab.RepositoryEntry{
			{Path: "go.mod", Type: "blob", Mode: "100644"},
			{Path: "internal/user/service.go", Type: "blob", Mode: "100644"},
			{Path: "linked.go", Type: "blob", Mode: "120000"},
		},
		files: map[string][]byte{
			"go.mod":                   []byte("module example.com/test\n"),
			"internal/user/service.go": []byte("package user\n"),
		},
	}
	manager := NewWorkspaceManager(source, WorkspaceOptions{})
	workspace, err := manager.Prepare(context.Background(), scm.Repository{Provider: "gitlab", ProviderProjectID: 123}, "head-sha", generation.GeneratedTest{
		FilePath: "internal/user/service_generated_test.go", Code: "package user\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	base := workspace.base
	assertFileContent(t, filepath.Join(workspace.Root, "go.mod"), "module example.com/test\n")
	assertFileContent(t, filepath.Join(workspace.Root, "internal/user/service_generated_test.go"), "package user\n")
	if _, err := os.Stat(filepath.Join(workspace.Root, "linked.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink entry was materialized: %v", err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("Cleanup() is not idempotent: %v", err)
	}
	if _, err := os.Stat(base); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace base still exists: %v", err)
	}
}

func TestWorkspaceManagerRejectsTargetCollisionAndUnsafeTreePath(t *testing.T) {
	tests := []struct {
		name    string
		entries []gitlab.RepositoryEntry
		wantErr error
	}{
		{name: "target collision", entries: []gitlab.RepositoryEntry{{
			Path: "internal/user/generated_test.go", Type: "blob", Mode: "100644",
		}}, wantErr: ErrGeneratedTargetExists},
		{name: "path traversal", entries: []gitlab.RepositoryEntry{{
			Path: "../outside.go", Type: "blob", Mode: "100644",
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWorkspaceManager(repositorySourceStub{entries: test.entries}, WorkspaceOptions{}).
				Prepare(context.Background(), scm.Repository{Provider: "gitlab", ProviderProjectID: 1}, "sha", generation.GeneratedTest{
					FilePath: "internal/user/generated_test.go", Code: "package user\n",
				})
			if err == nil || (test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("Prepare() error=%v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestWorkspaceManagerEnforcesFileAndByteLimits(t *testing.T) {
	source := repositorySourceStub{entries: []gitlab.RepositoryEntry{
		{Path: "one.go", Type: "blob"}, {Path: "two.go", Type: "blob"},
	}, files: map[string][]byte{"one.go": []byte("1234"), "two.go": []byte("5678")}}
	if _, err := NewWorkspaceManager(source, WorkspaceOptions{MaxFiles: 1, MaxBytes: 100}).
		Prepare(context.Background(), scm.Repository{Provider: "gitlab", ProviderProjectID: 1}, "sha", generation.GeneratedTest{
			FilePath: "generated_test.go", Code: "package test\n",
		}); err == nil {
		t.Fatal("Prepare() error=nil, want file limit error")
	}
	if _, err := NewWorkspaceManager(source, WorkspaceOptions{MaxFiles: 2, MaxBytes: 5}).
		Prepare(context.Background(), scm.Repository{Provider: "gitlab", ProviderProjectID: 1}, "sha", generation.GeneratedTest{
			FilePath: "generated_test.go", Code: "package test\n",
		}); err == nil {
		t.Fatal("Prepare() error=nil, want byte limit error")
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Fatalf("%s=%q, want %q", path, contents, want)
	}
}
