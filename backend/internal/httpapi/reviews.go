package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/review"
)

type ReviewService interface {
	Decide(ctx context.Context, generatedTestID int64, decision string, input review.DecisionInput) (review.Review, error)
	List(ctx context.Context, analysisID int64) ([]review.Review, error)
}

type reviewHandler struct{ service ReviewService }

func (h reviewHandler) list(w http.ResponseWriter, r *http.Request) {
	analysisID, ok := analysisID(w, r)
	if !ok {
		return
	}
	results, err := h.service.List(r.Context(), analysisID)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			writeError(w, http.StatusNotFound, "analysis not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not list review decisions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": results})
}

func (h reviewHandler) accept(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, review.DecisionAccepted)
}

func (h reviewHandler) reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, review.DecisionRejected)
}

func (h reviewHandler) decide(w http.ResponseWriter, r *http.Request, decision string) {
	generatedTestID, ok := generatedTestID(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input review.DecisionInput
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}
	result, err := h.service.Decide(r.Context(), generatedTestID, decision, input)
	if err != nil {
		switch {
		case errors.Is(err, job.ErrNotFound):
			writeError(w, http.StatusNotFound, "generated test not found")
		case errors.Is(err, review.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, review.ErrNotReady), errors.Is(err, review.ErrAlreadyReviewed), errors.Is(err, review.ErrStaleVersion):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "could not save review decision")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"review": result})
}

func generatedTestID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid generated test id")
		return 0, false
	}
	return id, true
}
