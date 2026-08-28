package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeadersAreApplied(t *testing.T) {
	response := httptest.NewRecorder()
	testRouter(checkerStub{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	for name, want := range map[string]string{
		"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff",
		"X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer",
	} {
		if got := response.Header().Get(name); got != want {
			t.Fatalf("%s=%q want=%q", name, got, want)
		}
	}
}

func TestRateLimiterRejectsBurstButKeepsHealthAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithPhaseElevenServices(logger, checkerStub{}, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil,
		RouterOptions{RateLimitPerSecond: .1, RateLimitBurst: 1, RateLimitMaxClients: 100})
	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodGet, path, nil)
		httpRequest.RemoteAddr = "192.0.2.10:1234"
		router.ServeHTTP(response, httpRequest)
		return response
	}
	if response := request("/unknown"); response.Code != http.StatusNotFound {
		t.Fatalf("first status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request("/unknown"); response.Code != http.StatusTooManyRequests ||
		response.Header().Get("Retry-After") == "" {
		t.Fatalf("limited status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if response := request("/health"); response.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRateLimiterDoesNotTrustForwardedAddress(t *testing.T) {
	limiter := newClientRateLimiter(RouterOptions{RateLimitPerSecond: 1, RateLimitBurst: 1, RateLimitMaxClients: 100})
	if !limiter.allow(remoteAddress("203.0.113.4:80"), testTime()) ||
		limiter.allow(remoteAddress("203.0.113.4:81"), testTime()) {
		t.Fatal("requests from the same socket host should share a bucket")
	}
}

func testTime() time.Time { return time.Unix(100, 0) }
