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

func TestAuthorizationMiddlewareEnforcesTokenAndRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := authorizationMiddleware(RouterOptions{AuthToken: "secret-token"}, next)
	tests := []struct {
		name, method, path, token, role string
		want                            int
	}{
		{name: "health bypass", method: http.MethodGet, path: "/health", want: http.StatusNoContent},
		{name: "missing token", method: http.MethodGet, path: "/api/document-sets", want: http.StatusUnauthorized},
		{name: "viewer read", method: http.MethodGet, path: "/api/document-sets", token: "secret-token", role: "viewer", want: http.StatusNoContent},
		{name: "viewer cannot upload", method: http.MethodPost, path: "/api/document-sets/1/documents", token: "secret-token", role: "viewer", want: http.StatusForbidden},
		{name: "editor uploads", method: http.MethodPost, path: "/api/document-sets/1/documents", token: "secret-token", role: "editor", want: http.StatusNoContent},
		{name: "editor cannot approve", method: http.MethodPost, path: "/api/document-versions/1/review", token: "secret-token", role: "editor", want: http.StatusForbidden},
		{name: "reviewer approves", method: http.MethodPost, path: "/api/document-versions/1/review", token: "secret-token", role: "reviewer", want: http.StatusNoContent},
		{name: "reviewer cannot preview purge", method: http.MethodGet, path: "/api/document-sets/1/purge", token: "secret-token", role: "reviewer", want: http.StatusForbidden},
		{name: "admin previews purge", method: http.MethodGet, path: "/api/document-sets/1/purge", token: "secret-token", role: "admin", want: http.StatusNoContent},
		{name: "reviewer cannot purge", method: http.MethodPost, path: "/api/document-sets/1/purge", token: "secret-token", role: "reviewer", want: http.StatusForbidden},
		{name: "admin repairs", method: http.MethodPost, path: "/api/test-run-items/1/repair", token: "secret-token", role: "admin", want: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			if test.role != "" {
				request.Header.Set("X-Authenticated-Role", test.role)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestLegacyDeprecationHeadersIdentifyOnlyCompatibilityRoutes(t *testing.T) {
	handler := legacyDeprecationHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		path       string
		deprecated bool
	}{
		{path: "/api/analyses/1/generated-tests", deprecated: true},
		{path: "/api/generated-tests/1/accept", deprecated: true},
		{path: "/api/evaluations", deprecated: true},
		{path: "/api/analyses/1/test-scope", deprecated: false},
		{path: "/api/document-sets/1/test-cases", deprecated: false},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		got := response.Header().Get("Deprecation") == "true"
		if got != test.deprecated {
			t.Fatalf("path=%s deprecated=%v want=%v", test.path, got, test.deprecated)
		}
		if test.deprecated && response.Header().Get("Link") == "" {
			t.Fatalf("path=%s has no successor Link header", test.path)
		}
	}
}
