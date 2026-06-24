// Package middleware provides HTTP middleware for Hekima's server.
//
// Middleware is applied in server.Run() and wraps the entire mux, so
// every current and future endpoint gets logging and rate limiting
// automatically without touching handler code.
package middleware

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// responseRecorder wraps http.ResponseWriter to capture the status code
// written by the handler, so the logging middleware can include it in
// the log line after the handler returns.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Logger logs every request on a single line:
//
//	2026/06/24 08:00:00 POST /chunk 200 142ms 192.168.1.1
//
// Fields: timestamp (from log package), method, path, status, duration, client IP.
// Logging happens after the handler returns so the status code is known.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s %s",
			r.Method,
			r.URL.Path,
			rec.status,
			time.Since(start).Round(time.Millisecond),
			clientIP(r),
		)
	})
}

// ipLimiter holds a rate limiter and the last time it was seen.
// lastSeen is used to evict stale entries from the limiter map.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter returns a middleware that limits each client IP to
// rps requests per second with the given burst size.
//
// Clients that exceed the limit receive 429 Too Many Requests with a
// JSON error body. The limiter map is cleaned up every minute to
// prevent unbounded memory growth from one-off client IPs.
//
// Recommended values for a document processing API:
//
//	rps=0.17 (10 per minute), burst=5
func RateLimiter(rps float64, burst int) func(http.Handler) http.Handler {
	var (
		mu       sync.Mutex
		limiters = make(map[string]*ipLimiter)
	)

	// Evict IPs not seen in the last 3 minutes. Runs in the background
	// so the map does not grow without bound in production.
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			for ip, l := range limiters {
				if time.Since(l.lastSeen) > 3*time.Minute {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := limiters[ip]
		if !ok {
			l = &ipLimiter{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
			limiters[ip] = l
		}
		l.lastSeen = time.Now()
		return l.limiter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !getLimiter(ip).Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded — maximum 10 requests per minute per IP"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the real client IP from the request, preferring
// X-Forwarded-For (set by reverse proxies and load balancers) over
// the direct remote address.
//
// Only the first entry in X-Forwarded-For is used — in a multi-hop
// proxy chain, the leftmost IP is the original client.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For may be "client, proxy1, proxy2"
		for i := 0; i < len(fwd); i++ {
			if fwd[i] == ',' {
				return fwd[:i]
			}
		}
		return fwd
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
