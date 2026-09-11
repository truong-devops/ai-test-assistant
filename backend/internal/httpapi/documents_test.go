package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/document"
)

type documentServiceStub struct {
	sets        []document.Set
	documents   []document.Document
	uploadInput document.UploadInput
	uploadBody  string
	err         error
}

func (s *documentServiceStub) CreateSet(_ context.Context, input document.CreateSetInput) (document.Set, error) {
	if s.err != nil {
		return document.Set{}, s.err
	}
	return document.Set{ID: 1, Name: input.Name, Description: input.Description, Status: document.SetStatusActive}, nil
}
func (s *documentServiceStub) ListSets(context.Context) ([]document.Set, error) { return s.sets, s.err }
func (s *documentServiceStub) GetSet(context.Context, int64) (document.Set, error) {
	if s.err != nil {
		return document.Set{}, s.err
	}
	return document.Set{ID: 1, Name: "Đặt hàng", Status: document.SetStatusActive}, nil
}
func (s *documentServiceStub) Upload(_ context.Context, setID int64, input document.UploadInput, source io.Reader) (document.Document, document.Version, error) {
	if s.err != nil {
		return document.Document{}, document.Version{}, s.err
	}
	payload, err := io.ReadAll(source)
	if err != nil {
		return document.Document{}, document.Version{}, err
	}
	s.uploadInput, s.uploadBody = input, string(payload)
	version := document.Version{ID: 3, DocumentID: 2, DocumentSetID: setID, VersionNumber: 1,
		ParseStatus: document.ParseUploaded, ApprovalStatus: document.ApprovalDraft}
	return document.Document{ID: 2, DocumentSetID: setID, Name: input.DocumentName}, version, nil
}
func (s *documentServiceStub) ListDocuments(context.Context, int64) ([]document.Document, error) {
	return s.documents, s.err
}
func (s *documentServiceStub) GetVersion(context.Context, int64, int) (document.Document, document.Version, []document.Block, error) {
	if s.err != nil {
		return document.Document{}, document.Version{}, nil, s.err
	}
	return document.Document{ID: 2}, document.Version{ID: 3, VersionNumber: 1},
		[]document.Block{{ID: 4, Content: "Nội dung"}}, nil
}

func TestDocumentHandlerCreatesSetWithStrictJSON(t *testing.T) {
	handler := documentHandler{service: &documentServiceStub{}, maxUploadBytes: 1024}
	request := httptest.NewRequest(http.MethodPost, "/api/document-sets",
		strings.NewReader(`{"name":"Đặt hàng","description":"URD"}`))
	response := httptest.NewRecorder()
	handler.createSet(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"name":"Đặt hàng"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/document-sets",
		strings.NewReader(`{"name":"Đặt hàng","unexpected":true}`))
	response = httptest.NewRecorder()
	handler.createSet(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDocumentHandlerAcceptsMultipartUpload(t *testing.T) {
	service := &documentServiceStub{}
	handler := documentHandler{service: service, maxUploadBytes: 1024}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("document_name", "Yêu cầu đặt hàng"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("document_type", "REQUIREMENTS"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "requirements.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "# Đặt hàng"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/document-sets/7/documents", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.SetPathValue("id", "7")
	response := httptest.NewRecorder()
	handler.upload(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.uploadInput.Filename != "requirements.md" || service.uploadInput.DocumentType != "REQUIREMENTS" ||
		service.uploadBody != "# Đặt hàng" {
		t.Fatalf("input=%+v body=%q", service.uploadInput, service.uploadBody)
	}
}

func TestDocumentHandlerMapsUploadAndLookupErrors(t *testing.T) {
	for name, testCase := range map[string]struct {
		err    error
		status int
	}{
		"not-found":   {document.ErrNotFound, http.StatusNotFound},
		"too-large":   {document.ErrFileTooLarge, http.StatusRequestEntityTooLarge},
		"unsupported": {document.ErrUnsupported, http.StatusBadRequest},
		"conflict":    {document.ErrAlreadyExists, http.StatusConflict},
		"internal":    {errors.New("database"), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeDocumentError(response, testCase.err, "fallback")
			if response.Code != testCase.status {
				t.Fatalf("status=%d, want %d", response.Code, testCase.status)
			}
		})
	}
}

func TestDocumentHandlerRejectsInvalidIDs(t *testing.T) {
	handler := documentHandler{service: &documentServiceStub{}, maxUploadBytes: 1024}
	request := httptest.NewRequest(http.MethodGet, "/api/documents/no/versions/0", nil)
	request.SetPathValue("id", "no")
	request.SetPathValue("version", "0")
	response := httptest.NewRecorder()
	handler.getVersion(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
