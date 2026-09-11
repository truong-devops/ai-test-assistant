package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLocalFileStoreRoundTripAndDelete(t *testing.T) {
	store, err := NewLocalFileStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("# Yêu cầu\nNội dung")
	stored, err := store.Save(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	if stored.SizeBytes != int64(len(payload)) || stored.SHA256 != hex.EncodeToString(hash[:]) ||
		!strings.HasPrefix(stored.StorageKey, "objects/") {
		t.Fatalf("stored=%+v", stored)
	}
	reader, err := store.Open(context.Background(), stored.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatalf("read=%q error=%v", actual, err)
	}
	if err := store.Delete(context.Background(), stored.StorageKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), stored.StorageKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("open deleted error=%v, want ErrNotFound", err)
	}
}

func TestLocalFileStoreRejectsInvalidContentAndTraversal(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileStore(root, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), bytes.NewReader(nil)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty error=%v", err)
	}
	if _, err := store.Save(context.Background(), strings.NewReader("12345")); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("large error=%v", err)
	}
	if _, err := store.Open(context.Background(), "../outside"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("traversal error=%v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files leaked: %v", entries)
	}
}

func TestLocalFileStoreHonorsCanceledContext(t *testing.T) {
	store, err := NewLocalFileStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Save(ctx, strings.NewReader("content")); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}
