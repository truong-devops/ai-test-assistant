package httpapi

import (
	"context"
	"net/http"
	"time"
)

type ReadinessChecker interface {
	Ping(ctx context.Context) error
}

type healthHandler struct {
	checker ReadinessChecker
}

func (h healthHandler) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h healthHandler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.checker.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
