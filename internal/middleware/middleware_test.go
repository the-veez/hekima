package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/the-veez/hekima/internal/middleware"
)

// okHandler is a trivial handler that always returns 200.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// TestLogger_CapturesStatusCode verifies that the logger middleware
// correctly captures non-200 status codes written by the handler.
// If responseRecorder.WriteHeader is broken, it would always log 200.
func TestLogger_CapturesStatusCode(t *testing.T) {
	handler := middleware.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 418
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("Logger: status = %d, want 418", rec.Code)
	}
}

// TestLogger_DefaultStatus200 verifies that if the handler never calls
// WriteHeader explicitly, the logger records 200 (the http default).
func TestLogger_DefaultStatus200(t *testing.T) {
	handler := middleware.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) // no explicit WriteHeader
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Logger default status: got %d, want 200", rec.Code)
	}
}

// TestRateLimiter_AllowsBurst verifies that up to burst requests
// are allowed immediately with no delay.
func TestRateLimiter_AllowsBurst(t *testing.T) {
	const burst = 3
	limiter := middleware.RateLimiter(0.001, burst) // near-zero rps so only burst tokens matter
	handler := limiter(okHandler)

	for i := 0; i < burst; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200 (within burst)", i+1, rec.Code)
		}
	}
}

// TestRateLimiter_BlocksAfterBurst verifies that requests beyond the
// burst limit are rejected with 429.
func TestRateLimiter_BlocksAfterBurst(t *testing.T) {
	const burst = 2
	limiter := middleware.RateLimiter(0.001, burst) // near-zero rps
	handler := limiter(okHandler)

	// Exhaust the burst.
	for i := 0; i < burst; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "192.0.2.2:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Next request must be rejected.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.2:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("post-burst request: status = %d, want 429", rec.Code)
	}
}

// TestRateLimiter_DifferentIPsAreIndependent verifies that rate limits
// are per-IP — exhausting one IP's bucket must not affect another IP.
func TestRateLimiter_DifferentIPsAreIndependent(t *testing.T) {
	const burst = 1
	limiter := middleware.RateLimiter(0.001, burst)
	handler := limiter(okHandler)

	// Exhaust IP A.
	for i := 0; i <= burst; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// IP B must still be allowed.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.11:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("IP B after IP A exhausted: status = %d, want 200", rec.Code)
	}
}

// TestRateLimiter_XForwardedForUsed verifies that X-Forwarded-For is
// preferred over RemoteAddr for IP extraction — critical for deployments
// behind a reverse proxy or load balancer where RemoteAddr is always
// the proxy's IP, not the real client.
func TestRateLimiter_XForwardedForUsed(t *testing.T) {
	const burst = 1
	limiter := middleware.RateLimiter(0.001, burst)
	handler := limiter(okHandler)

	// Two requests from the same RemoteAddr (proxy) but different
	// X-Forwarded-For values must be treated as different clients.
	makeReq := func(xff string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "10.0.0.1:80" // proxy IP — same for both
		req.Header.Set("X-Forwarded-For", xff)
		return req
	}

	// Client A — exhausts its burst.
	for i := 0; i <= burst; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, makeReq("203.0.113.1"))
	}

	// Client B — different XFF, must still have a full bucket.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, makeReq("203.0.113.2"))
	if rec.Code != http.StatusOK {
		t.Errorf("client B (different XFF): status = %d, want 200", rec.Code)
	}
}
