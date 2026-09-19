package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

func TestTestCaseRevisionErrorsUseStructuredStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{name: "stale head", err: &testcase.RevisionConflictError{
			CurrentRevisionID: 42, CurrentHeadToken: "head-token"},
			want: http.StatusConflict, code: "TESTCASE_HEAD_CONFLICT"},
		{name: "idempotency collision", err: testcase.ErrIdempotencyConflict,
			want: http.StatusConflict, code: "IDEMPOTENCY_KEY_REUSED"},
		{name: "foreign evidence", err: testcase.ErrEvidenceInvalid,
			want: http.StatusUnprocessableEntity, code: "TESTCASE_EVIDENCE_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeWorkflowError(response, test.err, "fallback")
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code {
				t.Fatalf("code=%q want=%q", body.Code, test.code)
			}
		})
	}
}
