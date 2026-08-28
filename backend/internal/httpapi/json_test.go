package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONReturnsValidErrorWhenValueCannotBeEncoded(t *testing.T) {
	response := httptest.NewRecorder()
	writeJSON(response, http.StatusOK, map[string]float64{"score": math.NaN()})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil ||
		body["error"] != "could not encode JSON response" {
		t.Fatalf("body=%q error=%v", response.Body.String(), err)
	}
}
