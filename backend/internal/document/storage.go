package document

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileStore interface {
	Save(context.Context, io.Reader) (StoredFile, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type LocalFileStore struct {
	root     string
	maxBytes int64
}

func NewLocalFileStore(root string, maxBytes int64) (*LocalFileStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("document storage path is required")
	}
	if maxBytes <= 0 || maxBytes > 256<<20 {
		return nil, fmt.Errorf("document upload limit must be between 1 and 268435456 bytes")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve document storage path: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create document storage path: %w", err)
	}
	return &LocalFileStore{root: absolute, maxBytes: maxBytes}, nil
}

func (s *LocalFileStore) Save(ctx context.Context, source io.Reader) (StoredFile, error) {
	if source == nil {
		return StoredFile{}, fmt.Errorf("%w: document file is required", ErrInvalidInput)
	}
	temporary, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return StoredFile{}, fmt.Errorf("create temporary document: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	hasher := sha256.New()
	written, err := copyWithContext(ctx, io.MultiWriter(temporary, hasher), source, s.maxBytes+1)
	if err != nil {
		return StoredFile{}, fmt.Errorf("store document: %w", err)
	}
	if written == 0 {
		return StoredFile{}, fmt.Errorf("%w: document file is empty", ErrInvalidInput)
	}
	if written > s.maxBytes {
		return StoredFile{}, ErrFileTooLarge
	}
	if err := temporary.Sync(); err != nil {
		return StoredFile{}, fmt.Errorf("sync document: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return StoredFile{}, fmt.Errorf("close document: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return StoredFile{}, fmt.Errorf("secure document permissions: %w", err)
	}

	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return StoredFile{}, fmt.Errorf("generate document storage key: %w", err)
	}
	key := filepath.Join("objects", hex.EncodeToString(identifier[:1]), hex.EncodeToString(identifier))
	destination, err := s.resolve(key)
	if err != nil {
		return StoredFile{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return StoredFile{}, fmt.Errorf("create document object directory: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return StoredFile{}, fmt.Errorf("commit document object: %w", err)
	}
	committed = true
	return StoredFile{StorageKey: filepath.ToSlash(key), SizeBytes: written,
		SHA256: hex.EncodeToString(hasher.Sum(nil))}, nil
}

func (s *LocalFileStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open document object: %w", err)
	}
	return file, nil
}

func (s *LocalFileStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete document object: %w", err)
	}
	return nil
}

func (s *LocalFileStore) resolve(key string) (string, error) {
	key = filepath.FromSlash(strings.TrimSpace(key))
	cleaned := filepath.Clean(key)
	if key == "" || cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: invalid document storage key", ErrInvalidInput)
	}
	result := filepath.Join(s.root, cleaned)
	relative, err := filepath.Rel(s.root, result)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: invalid document storage key", ErrInvalidInput)
	}
	return result, nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader, limit int64) (int64, error) {
	buffer := make([]byte, 32<<10)
	limited := io.LimitReader(source, limit)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := limited.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
