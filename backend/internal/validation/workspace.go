package validation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/generation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/gitlab"
)

var ErrGeneratedTargetExists = errors.New("generated test target already exists in repository")

type RepositorySource interface {
	ListRepositoryTree(ctx context.Context, projectID int64, ref string) ([]gitlab.RepositoryEntry, error)
	GetFileRaw(ctx context.Context, projectID int64, filePath, ref string) ([]byte, error)
}

type WorkspaceOptions struct {
	MaxFiles int
	MaxBytes int64
}

type WorkspaceManager struct {
	source   RepositorySource
	maxFiles int
	maxBytes int64
}

func NewWorkspaceManager(source RepositorySource, options WorkspaceOptions) *WorkspaceManager {
	maxFiles := options.MaxFiles
	if maxFiles <= 0 {
		maxFiles = DefaultMaxRepositoryFiles
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxRepositoryBytes
	}
	return &WorkspaceManager{source: source, maxFiles: maxFiles, maxBytes: maxBytes}
}

type Workspace struct {
	Root string
	base string
	once sync.Once
	err  error
}

func (w *Workspace) Cleanup() error {
	w.once.Do(func() { w.err = os.RemoveAll(w.base) })
	return w.err
}

func (m *WorkspaceManager) Prepare(ctx context.Context, projectID int64, ref string,
	generated generation.GeneratedTest,
) (*Workspace, error) {
	if projectID <= 0 || strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("workspace project ID and ref are required")
	}
	target, err := safeRepositoryPath(generated.FilePath)
	if err != nil {
		return nil, fmt.Errorf("invalid generated test path: %w", err)
	}
	entries, err := m.source.ListRepositoryTree(ctx, projectID, ref)
	if err != nil {
		return nil, fmt.Errorf("list validation repository snapshot: %w", err)
	}
	blobs := make([]gitlab.RepositoryEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entryPath, err := safeRepositoryPath(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("unsafe repository entry path: %w", err)
		}
		if _, exists := seen[entryPath]; exists {
			return nil, fmt.Errorf("duplicate repository entry %q", entryPath)
		}
		seen[entryPath] = struct{}{}
		if entryPath == target {
			return nil, fmt.Errorf("%w: %s", ErrGeneratedTargetExists, target)
		}
		if entry.Type == "blob" && entry.Mode != "120000" {
			blobs = append(blobs, entry)
		}
	}
	if len(blobs) > m.maxFiles {
		return nil, fmt.Errorf("repository snapshot has %d files; limit is %d", len(blobs), m.maxFiles)
	}

	base, err := os.MkdirTemp("", "ai-test-validation-")
	if err != nil {
		return nil, fmt.Errorf("create validation workspace: %w", err)
	}
	workspace := &Workspace{Root: filepath.Join(base, "workspace"), base: base}
	failed := true
	defer func() {
		if failed {
			_ = workspace.Cleanup()
		}
	}()
	if err := os.Mkdir(workspace.Root, 0o777); err != nil {
		return nil, fmt.Errorf("create validation workspace root: %w", err)
	}
	if err := os.Chmod(workspace.Root, 0o777); err != nil {
		return nil, fmt.Errorf("set validation workspace permissions: %w", err)
	}

	totalBytes := int64(len(generated.Code))
	if totalBytes > m.maxBytes {
		return nil, fmt.Errorf("repository snapshot with generated test exceeds %d bytes", m.maxBytes)
	}
	for _, entry := range blobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contents, err := m.source.GetFileRaw(ctx, projectID, entry.Path, ref)
		if err != nil {
			return nil, fmt.Errorf("fetch validation repository file %q: %w", entry.Path, err)
		}
		totalBytes += int64(len(contents))
		if totalBytes > m.maxBytes {
			return nil, fmt.Errorf("repository snapshot exceeds %d bytes", m.maxBytes)
		}
		if err := writeWorkspaceFile(workspace.Root, entry.Path, contents); err != nil {
			return nil, err
		}
	}
	if err := writeWorkspaceFile(workspace.Root, target, []byte(generated.Code)); err != nil {
		return nil, fmt.Errorf("write generated test: %w", err)
	}
	failed = false
	return workspace, nil
}

func writeWorkspaceFile(root, repositoryPath string, contents []byte) error {
	clean, err := safeRepositoryPath(repositoryPath)
	if err != nil {
		return err
	}
	destination := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(root, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("repository path escapes validation workspace")
	}
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o777); err != nil {
		return fmt.Errorf("create validation directory for %q: %w", clean, err)
	}
	if err := chmodPathToRoot(root, directory, 0o777); err != nil {
		return err
	}
	if err := os.WriteFile(destination, contents, 0o666); err != nil {
		return fmt.Errorf("write validation repository file %q: %w", clean, err)
	}
	if err := os.Chmod(destination, 0o666); err != nil {
		return fmt.Errorf("set validation file permissions for %q: %w", clean, err)
	}
	return nil
}

func chmodPathToRoot(root, directory string, mode os.FileMode) error {
	for current := directory; current != root; current = filepath.Dir(current) {
		if err := os.Chmod(current, mode); err != nil {
			return fmt.Errorf("set validation directory permissions: %w", err)
		}
	}
	return nil
}

func safeRepositoryPath(value string) (string, error) {
	if value == "" || len(value) > 512 || strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("invalid relative repository path")
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid relative repository path")
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid relative repository path")
		}
	}
	return clean, nil
}
