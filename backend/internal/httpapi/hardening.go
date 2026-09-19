package httpapi

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RouterOptions struct {
	RateLimitPerSecond  float64
	RateLimitBurst      int
	RateLimitMaxClients int
	AuthToken           string
}

func authorizationMiddleware(options RouterOptions, next http.Handler) http.Handler {
	token := strings.TrimSpace(options.AuthToken)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || strings.HasPrefix(r.URL.Path, "/api/webhooks/") {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		role := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Authenticated-Role")))
		if role == "" {
			role = "viewer"
		}
		if !roleAllowed(role, requiredRole(r)) {
			writeError(w, http.StatusForbidden, "insufficient role")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// legacyDeprecationHeaders keeps historical read/review routes available during
// migration while making their compatibility status machine-readable. The
// successor is the document-driven API rooted at /api/document-sets.
func legacyDeprecationHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		legacy := strings.HasPrefix(path, "/api/evaluations") ||
			strings.HasPrefix(path, "/api/generated-tests/") ||
			(strings.HasPrefix(path, "/api/analyses/") &&
				containsAny(path, "/recommendations", "/generated-tests", "/validations", "/repairs")) ||
			(r.Method == http.MethodPost && strings.HasPrefix(path, "/api/document-sets/") &&
				(strings.HasSuffix(path, "/index") || strings.HasSuffix(path, "/requirements/extract") ||
					strings.HasSuffix(path, "/test-cases/generate") ||
					strings.HasSuffix(path, "/test-cases/regenerate")))
		if legacy {
			w.Header().Set("Deprecation", "true")
			if strings.HasPrefix(path, "/api/document-sets/") {
				remainder := strings.TrimPrefix(path, "/api/document-sets/")
				setID, _, _ := strings.Cut(remainder, "/")
				w.Header().Set("Link", "</api/document-sets/"+setID+
					`/workflow-operations>; rel="successor-version"`)
			} else {
				w.Header().Set("Link", `</api/document-sets>; rel="successor-version"`)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func requiredRole(r *http.Request) string {
	if strings.Contains(r.URL.Path, "/purge") {
		return "admin"
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return "viewer"
	}
	path := r.URL.Path
	for _, marker := range []string{"/review", "/classification", "/execute", "/exports", "/repair", "/lifecycle", "/archive", "/pipeline-mode", "/document-baseline", "/test-scope", "/suite-releases"} {
		if strings.Contains(path, marker) {
			return "reviewer"
		}
	}
	return "editor"
}

func roleAllowed(actual, required string) bool {
	ranks := map[string]int{"viewer": 1, "editor": 2, "reviewer": 3, "admin": 4}
	return ranks[actual] >= ranks[required]
}

type rateLimitClient struct {
	tokens   float64
	lastSeen time.Time
}

type clientRateLimiter struct {
	mu         sync.Mutex
	rate       float64
	burst      float64
	maxClients int
	clients    map[string]rateLimitClient
	requests   uint64
}

func newClientRateLimiter(options RouterOptions) *clientRateLimiter {
	return &clientRateLimiter{rate: options.RateLimitPerSecond, burst: float64(options.RateLimitBurst),
		maxClients: options.RateLimitMaxClients, clients: make(map[string]rateLimitClient)}
}

func (l *clientRateLimiter) enabled() bool {
	return l.rate > 0 && l.burst >= 1 && l.maxClients > 0
}

func (l *clientRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests++
	if l.requests%256 == 0 {
		l.prune(now)
	}
	client, exists := l.clients[key]
	if !exists {
		if len(l.clients) >= l.maxClients {
			l.prune(now)
		}
		if len(l.clients) >= l.maxClients {
			return false
		}
		client = rateLimitClient{tokens: l.burst, lastSeen: now}
	}
	elapsed := now.Sub(client.lastSeen).Seconds()
	if elapsed > 0 {
		client.tokens = min(l.burst, client.tokens+elapsed*l.rate)
	}
	client.lastSeen = now
	allowed := client.tokens >= 1
	if allowed {
		client.tokens--
	}
	l.clients[key] = client
	return allowed
}

func (l *clientRateLimiter) prune(now time.Time) {
	cutoff := now.Add(-10 * time.Minute)
	for key, client := range l.clients {
		if client.lastSeen.Before(cutoff) {
			delete(l.clients, key)
		}
	}
}

func (l *clientRateLimiter) middleware(next http.Handler) http.Handler {
	if !l.enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}
		if !l.allow(remoteAddress(r.RemoteAddr), time.Now()) {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(1/l.rate))))
			writeError(w, http.StatusTooManyRequests, "request rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func remoteAddress(value string) string {
	host, _, err := net.SplitHostPort(value)
	if err == nil && host != "" {
		return host
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
