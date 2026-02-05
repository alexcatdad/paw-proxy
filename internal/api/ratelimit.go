package api

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a fixed-window rate limiter.
// It tracks the number of requests in the current 1-second window
// and rejects requests that exceed the configured limit.
//
// SECURITY: Rate limiting prevents runaway scripts from causing unbounded
// route map growth or CPU-intensive cert generation via the unix socket API.
type rateLimiter struct {
	mu          sync.Mutex
	limit       int
	count       int
	windowStart time.Time
}

func newRateLimiter(requestsPerSecond int) *rateLimiter {
	return &rateLimiter{
		limit:       requestsPerSecond,
		windowStart: time.Now(),
	}
}

// allow checks if a request is allowed under the rate limit.
// Returns true if the request is allowed, false if rate limited.
func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if now.Sub(rl.windowStart) >= time.Second {
		rl.count = 0
		rl.windowStart = now
	}

	if rl.count >= rl.limit {
		return false
	}

	rl.count++
	return true
}

// rateLimit wraps an http.HandlerFunc with rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded.
func rateLimit(limiter *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow() {
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}
