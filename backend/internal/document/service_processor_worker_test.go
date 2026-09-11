package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type repositoryStub struct {
	set              Set
	createdInput     UploadInput
	createdStored    StoredFile
	createVersionErr error
	blocks           []Block
}

func (r *repositoryStub) CreateSet(_ context.Context, input CreateSetInput) (Set, error) {
	r.set = Set{ID: 1, Name: input.Name, ProductName: input.ProductName, Scope: input.Scope,
		Description: input.Description, Status: SetStatusActive}
	return r.set, nil
}

func TestServiceCreateSetNormalizesProductAndScope(t *testing.T) {
	repository := &repositoryStub{}
	created, err := NewService(repository, &fileStoreStub{}).CreateSet(context.Background(), CreateSetInput{
		Name: " Checkout v1 ", ProductName: " Storefront ", Scope: " UC-B08 ", Description: " Order flow ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Checkout v1" || created.ProductName != "Storefront" ||
		created.Scope != "UC-B08" || created.Description != "Order flow" {
		t.Fatalf("created=%+v", created)
	}
}
func (r *repositoryStub) ListSets(context.Context) ([]Set, error) { return []Set{r.set}, nil }
func (r *repositoryStub) GetSet(context.Context, int64) (Set, error) {
	if r.set.ID == 0 {
		return Set{}, ErrNotFound
	}
	return r.set, nil
}
func (r *repositoryStub) CreateVersion(_ context.Context, setID int64, input UploadInput, stored StoredFile) (Document, Version, error) {
	r.createdInput, r.createdStored = input, stored
	if r.createVersionErr != nil {
		return Document{}, Version{}, r.createVersionErr
	}
	version := Version{ID: 3, DocumentID: 2, DocumentSetID: setID, VersionNumber: 1,
		OriginalFilename: input.Filename, MediaType: input.MediaType, ApprovalStatus: input.ApprovalStatus,
		SizeBytes: stored.SizeBytes, SHA256: stored.SHA256, StorageKey: stored.StorageKey,
		ParseStatus: ParseUploaded}
	return Document{ID: 2, DocumentSetID: setID, Name: input.DocumentName,
		DocumentType: input.DocumentType, LatestVersion: &version}, version, nil
}
func (r *repositoryStub) ListDocuments(context.Context, int64) ([]Document, error) { return nil, nil }
func (r *repositoryStub) GetVersion(context.Context, int64, int) (Document, Version, error) {
	return Document{ID: 2}, Version{ID: 3}, nil
}
func (r *repositoryStub) ListBlocks(context.Context, int64) ([]Block, error) { return r.blocks, nil }

type fileStoreStub struct {
	payload []byte
	stored  StoredFile
	deleted []string
	saveErr error
	openErr error
}

func (s *fileStoreStub) Save(_ context.Context, source io.Reader) (StoredFile, error) {
	if s.saveErr != nil {
		return StoredFile{}, s.saveErr
	}
	payload, err := io.ReadAll(source)
	if err != nil {
		return StoredFile{}, err
	}
	s.payload = payload
	hash := sha256.Sum256(payload)
	s.stored = StoredFile{StorageKey: "objects/test", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(hash[:])}
	return s.stored, nil
}
func (s *fileStoreStub) Open(context.Context, string) (io.ReadCloser, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.payload)), nil
}
func (s *fileStoreStub) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func TestServiceUploadNormalizesInputAndStartsDraftVersion(t *testing.T) {
	repository := &repositoryStub{set: Set{ID: 1, Status: SetStatusActive}}
	files := &fileStoreStub{}
	service := NewService(repository, files)
	item, version, err := service.Upload(context.Background(), 1, UploadInput{
		Filename: "yeu-cau.MD", DocumentType: " requirements ",
	}, strings.NewReader("# Đặt hàng"))
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "yeu-cau" || repository.createdInput.MediaType != MediaTypeMarkdown ||
		repository.createdInput.DocumentType != TypeRequirements || version.ApprovalStatus != ApprovalDraft {
		t.Fatalf("document=%+v version=%+v input=%+v", item, version, repository.createdInput)
	}
}

func TestServiceUploadRejectsUnsupportedFilesBeforeStorage(t *testing.T) {
	repository := &repositoryStub{set: Set{ID: 1, Status: SetStatusActive}}
	files := &fileStoreStub{}
	_, _, err := NewService(repository, files).Upload(context.Background(), 1,
		UploadInput{Filename: "requirements.pdf"}, strings.NewReader("pdf"))
	if !errors.Is(err, ErrUnsupported) || files.stored.StorageKey != "" {
		t.Fatalf("error=%v stored=%+v", err, files.stored)
	}
}

func TestServiceUploadDeletesObjectWhenMetadataCommitFails(t *testing.T) {
	repository := &repositoryStub{set: Set{ID: 1, Status: SetStatusActive}, createVersionErr: errors.New("database unavailable")}
	files := &fileStoreStub{}
	_, _, err := NewService(repository, files).Upload(context.Background(), 1,
		UploadInput{Filename: "requirements.md"}, strings.NewReader("content"))
	if err == nil || len(files.deleted) != 1 || files.deleted[0] != "objects/test" {
		t.Fatalf("error=%v deleted=%v", err, files.deleted)
	}
}

type resultSaverStub struct {
	blocks []ParsedBlock
	err    error
}

func (s *resultSaverStub) SaveParsed(_ context.Context, _ Version, blocks []ParsedBlock) error {
	s.blocks = blocks
	return s.err
}

func TestProcessorVerifiesMetadataAndPersistsBlocks(t *testing.T) {
	payload := []byte("# Thanh toán\n\nThẻ hợp lệ")
	hash := sha256.Sum256(payload)
	files := &fileStoreStub{payload: payload}
	saver := &resultSaverStub{}
	processor := NewProcessor(files, NewStructuredParser(), saver)
	version := Version{ID: 1, DocumentID: 2, SizeBytes: int64(len(payload)),
		SHA256: hex.EncodeToString(hash[:]), StorageKey: "objects/test",
		MediaType: MediaTypeMarkdown, OriginalFilename: "requirements.md"}
	if err := processor.Process(context.Background(), version); err != nil {
		t.Fatal(err)
	}
	if len(saver.blocks) != 2 || saver.blocks[0].BlockType != BlockHeading {
		t.Fatalf("blocks=%+v", saver.blocks)
	}
}

func TestProcessorRejectsChecksumMismatch(t *testing.T) {
	payload := []byte("content")
	processor := NewProcessor(&fileStoreStub{payload: payload}, NewStructuredParser(), &resultSaverStub{})
	err := processor.Process(context.Background(), Version{ID: 1, DocumentID: 2,
		SizeBytes: int64(len(payload)), SHA256: strings.Repeat("0", 64),
		StorageKey: "objects/test", MediaType: MediaTypeMarkdown, OriginalFilename: "input.md"})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("error=%v, want checksum error", err)
	}
}

type parseQueueStub struct {
	claimed    Version
	claimErr   error
	retryCalls int
	retryErr   error
}

func (q *parseQueueStub) ClaimNext(context.Context, time.Duration) (Version, error) {
	return q.claimed, q.claimErr
}
func (q *parseQueueStub) RenewLease(context.Context, Version, time.Duration) error { return nil }
func (q *parseQueueStub) RetryOrFail(_ context.Context, _ Version, _ error, _ int, _ time.Duration) error {
	q.retryCalls++
	return q.retryErr
}

type versionProcessorStub struct{ err error }

func (p versionProcessorStub) Process(context.Context, Version) error { return p.err }

func TestWorkerRunOnceRetriesProcessingFailure(t *testing.T) {
	queue := &parseQueueStub{claimed: Version{ID: 1, DocumentID: 2, AttemptCount: 1}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := NewWorker(logger, queue, versionProcessorStub{err: errors.New("invalid document")}, WorkerOptions{
		PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute,
		ParseTimeout: time.Second, MaxAttempts: 3,
	})
	if err := worker.runOnce(context.Background()); err == nil || queue.retryCalls != 1 {
		t.Fatalf("error=%v retryCalls=%d", err, queue.retryCalls)
	}
}

func TestWorkerRunOnceTreatsEmptyQueueAsSuccess(t *testing.T) {
	queue := &parseQueueStub{claimErr: ErrNotFound}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), queue, versionProcessorStub{}, WorkerOptions{
		PollInterval: time.Second, RetryDelay: time.Second, LeaseDuration: time.Minute,
		ParseTimeout: time.Second, MaxAttempts: 3,
	})
	if err := worker.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}
