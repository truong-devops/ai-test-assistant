package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/testcase"
)

type testcaseProposalService interface {
	ListProposals(context.Context, int64, int64, int64, int) (testcase.ProposalPage, error)
	DecideProposal(context.Context, int64, testcase.ProposalDecision, string, string) (testcase.GenerationProposal, error)
}

func (h testCaseWorkflowHandler) compareProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "proposal")
	if !ok {
		return
	}
	familyID, err := strconv.ParseInt(r.URL.Query().Get("target_family_id"), 10, 64)
	if err != nil || familyID <= 0 {
		writeError(w, 400, "invalid target family")
		return
	}
	svc, ok := h.service.(interface {
		CompareProposal(context.Context, int64, int64) (testcase.ProposalComparison, error)
	})
	if !ok {
		writeError(w, 501, "proposal comparison unavailable")
		return
	}
	result, err := svc.CompareProposal(r.Context(), id, familyID)
	if err != nil {
		writeWorkflowError(w, err, "could not compare proposal")
		return
	}
	writeJSON(w, 200, result)
}

func (h testCaseWorkflowHandler) proposals(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "document set")
	if !ok {
		return
	}
	svc, ok := h.service.(testcaseProposalService)
	if !ok {
		writeError(w, 501, "proposal service unavailable")
		return
	}
	values := map[string]int64{"job_id": 0, "before": 0, "limit": 20}
	for key := range values {
		if value := r.URL.Query().Get(key); value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				writeError(w, 400, "invalid proposal pagination")
				return
			}
			values[key] = n
		}
	}
	if values["limit"] < 1 || values["limit"] > 50 {
		writeError(w, 400, "limit must be 1–50")
		return
	}
	result, err := svc.ListProposals(r.Context(), id, values["job_id"], values["before"], int(values["limit"]))
	if err != nil {
		writeWorkflowError(w, err, "could not list testcase proposals")
		return
	}
	writeJSON(w, 200, result)
}

func (h testCaseWorkflowHandler) decideProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveInt64Path(w, r, "id", "proposal")
	if !ok {
		return
	}
	svc, ok := h.service.(testcaseProposalService)
	if !ok {
		writeError(w, 501, "proposal service unavailable")
		return
	}
	var input testcase.ProposalDecision
	if !decodeWorkflowJSON(w, r, &input) {
		return
	}
	result, err := svc.DecideProposal(r.Context(), id, input, r.Header.Get("Idempotency-Key"), workflowActor(r))
	if err != nil {
		writeWorkflowError(w, err, "could not review testcase proposal")
		return
	}
	writeJSON(w, 200, result)
}
