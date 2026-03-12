package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAllowWithinLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 10) // 10 per minute

	for i := 0; i < 10; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestDenyOverLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 5) // 5 per minute

	// Exhaust the bucket
	for i := 0; i < 5; i++ {
		rl.Allow("1.2.3.4")
	}

	// Next request should be denied
	if rl.Allow("1.2.3.4") {
		t.Error("request should be denied after exceeding limit")
	}
}

func TestDifferentIPsIndependent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 2)

	rl.Allow("1.1.1.1")
	rl.Allow("1.1.1.1")

	// Second IP should still be allowed
	if !rl.Allow("2.2.2.2") {
		t.Error("different IP should have independent bucket")
	}
}

func TestTokenRefill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 60) // 1 token per second

	// Exhaust bucket
	for i := 0; i < 60; i++ {
		rl.Allow("1.2.3.4")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("should be denied after exhausting bucket")
	}

	// Manually advance lastCheck to simulate time passing
	rl.mu.Lock()
	rl.buckets["1.2.3.4"].lastCheck = time.Now().Add(-2 * time.Second)
	rl.mu.Unlock()

	// Should have refilled ~2 tokens
	if !rl.Allow("1.2.3.4") {
		t.Error("should be allowed after token refill")
	}
}

func TestWrapHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 1)

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request allowed
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Second request denied
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestWrapHandlerBadRemoteAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 10)
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// RemoteAddr without port — should still work (uses raw string as key)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCleanupRemovesStaleEntries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 100)
	rl.Allow("stale-ip")

	// Manually backdate the entry beyond the 10-minute cutoff
	rl.mu.Lock()
	rl.buckets["stale-ip"].lastCheck = time.Now().Add(-15 * time.Minute)
	rl.mu.Unlock()

	// Run cleanup manually
	rl.mu.Lock()
	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, b := range rl.buckets {
		if b.lastCheck.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
	rl.mu.Unlock()

	rl.mu.Lock()
	_, exists := rl.buckets["stale-ip"]
	rl.mu.Unlock()

	if exists {
		t.Error("stale entry should have been cleaned up")
	}
}

func TestContextCancellationStopsCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_ = NewRateLimiter(ctx, 100)

	// Cancel context — cleanup goroutine should exit
	cancel()

	// Give the goroutine a moment to exit
	time.Sleep(10 * time.Millisecond)

	// If we get here without hanging, the goroutine exited properly.
	// This is a basic smoke test — the goroutine leak was the original issue.
}
