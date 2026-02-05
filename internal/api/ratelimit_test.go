package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimiterAllowWithinLimit(t *testing.T) {
	rl := newRateLimiter(5)

	for i := 0; i < 5; i++ {
		if !rl.allow() {
			t.Errorf("request %d should be allowed (limit is 5)", i+1)
		}
	}
}

func TestRateLimiterRejectOverLimit(t *testing.T) {
	rl := newRateLimiter(3)

	// Use up all 3 allowed requests
	for i := 0; i < 3; i++ {
		if !rl.allow() {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be rejected
	if rl.allow() {
		t.Error("request 4 should be rejected (limit is 3)")
	}

	// 5th request should also be rejected
	if rl.allow() {
		t.Error("request 5 should be rejected (limit is 3)")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	rl := newRateLimiter(2)

	// Use up all allowed requests
	for i := 0; i < 2; i++ {
		if !rl.allow() {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// Should be rejected now
	if rl.allow() {
		t.Error("request should be rejected before window reset")
	}

	// Manually advance the window by setting windowStart in the past
	rl.mu.Lock()
	rl.windowStart = time.Now().Add(-2 * time.Second)
	rl.mu.Unlock()

	// Should be allowed after window reset
	if !rl.allow() {
		t.Error("request should be allowed after window reset")
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	rl := newRateLimiter(100)

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// Launch 200 goroutines all trying to get through at once
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- rl.allow()
		}()
	}

	wg.Wait()
	close(allowed)

	allowedCount := 0
	rejectedCount := 0
	for result := range allowed {
		if result {
			allowedCount++
		} else {
			rejectedCount++
		}
	}

	if allowedCount != 100 {
		t.Errorf("expected exactly 100 allowed, got %d", allowedCount)
	}
	if rejectedCount != 100 {
		t.Errorf("expected exactly 100 rejected, got %d", rejectedCount)
	}
}

func TestRateLimiterSingleRequest(t *testing.T) {
	rl := newRateLimiter(1)

	if !rl.allow() {
		t.Error("first request should be allowed")
	}

	if rl.allow() {
		t.Error("second request should be rejected (limit is 1)")
	}
}

func TestRateLimitMiddleware429(t *testing.T) {
	rl := newRateLimiter(1)

	handler := rateLimit(rl, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request should succeed
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected status 200, got %d", rec.Code)
	}

	// Second request should be rate limited
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	rec = httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected status 429, got %d", rec.Code)
	}

	// Verify the response body contains the error message
	body := rec.Body.String()
	if body == "" {
		t.Error("expected JSON error body, got empty response")
	}
}

func TestRateLimitMiddlewarePassesThrough(t *testing.T) {
	rl := newRateLimiter(10)
	called := false

	handler := rateLimit(rl, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("expected handler to be called when under rate limit")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}
}

func TestRateLimitMiddleware429ContentType(t *testing.T) {
	rl := newRateLimiter(0) // limit of 0 means all requests are rejected

	handler := rateLimit(rl, func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when rate limited")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", contentType)
	}
}
